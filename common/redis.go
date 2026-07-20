package common

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

var RDB *redis.Client
var RedisEnabled = true

func RedisKeyCacheSeconds() int {
	return SyncFrequency
}

// InitRedisClient This function is called after init()
func InitRedisClient() (err error) {
	if os.Getenv("REDIS_CONN_STRING") == "" {
		RedisEnabled = false
		SysLog("REDIS_CONN_STRING not set, Redis is not enabled")
		return nil
	}
	if os.Getenv("SYNC_FREQUENCY") == "" {
		SysLog("SYNC_FREQUENCY not set, use default value 60")
		SyncFrequency = 60
	}
	SysLog("Redis is enabled")
	opt, err := redis.ParseURL(os.Getenv("REDIS_CONN_STRING"))
	if err != nil {
		FatalLog("failed to parse Redis connection string: " + err.Error())
	}
	opt.PoolSize = GetEnvOrDefault("REDIS_POOL_SIZE", 10)
	RDB = redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = RDB.Ping(ctx).Result()
	if err != nil {
		FatalLog("Redis ping test failed: " + err.Error())
	}
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis connected to %s", opt.Addr))
		SysLog(fmt.Sprintf("Redis database: %d", opt.DB))
	}
	return err
}

func ParseRedisOption() *redis.Options {
	opt, err := redis.ParseURL(os.Getenv("REDIS_CONN_STRING"))
	if err != nil {
		FatalLog("failed to parse Redis connection string: " + err.Error())
	}
	return opt
}

func RedisSet(key string, value string, expiration time.Duration) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis SET: key=%s, value=%s, expiration=%v", key, value, expiration))
	}
	ctx := context.Background()
	return RDB.Set(ctx, key, value, expiration).Err()
}

func RedisGet(key string) (string, error) {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis GET: key=%s", key))
	}
	ctx := context.Background()
	val, err := RDB.Get(ctx, key).Result()
	return val, err
}

//func RedisExpire(key string, expiration time.Duration) error {
//	ctx := context.Background()
//	return RDB.Expire(ctx, key, expiration).Err()
//}
//
//func RedisGetEx(key string, expiration time.Duration) (string, error) {
//	ctx := context.Background()
//	return RDB.GetSet(ctx, key, expiration).Result()
//}

func RedisDel(key string) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis DEL: key=%s", key))
	}
	ctx := context.Background()
	return RDB.Del(ctx, key).Err()
}

func RedisDelKey(key string) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis DEL Key: key=%s", key))
	}
	ctx := context.Background()
	return RDB.Del(ctx, key).Err()
}

func RedisDelKeys(keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if RDB == nil {
		return errors.New("redis client is nil")
	}
	ctx := context.Background()
	return RDB.Del(ctx, keys...).Err()
}

func redisHashObjectData(obj interface{}) (map[string]interface{}, error) {
	data := make(map[string]interface{})
	// 使用反射遍历结构体字段
	valueOfObj := reflect.ValueOf(obj)
	if valueOfObj.Kind() != reflect.Ptr || valueOfObj.IsNil() {
		return nil, fmt.Errorf("obj must be a non-nil pointer to a struct, got %T", obj)
	}
	v := valueOfObj.Elem()
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("obj must be a pointer to a struct, got pointer to %T", v.Interface())
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		// Skip DeletedAt field
		if field.Type.String() == "gorm.DeletedAt" {
			continue
		}

		// 处理指针类型
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				data[field.Name] = ""
				continue
			}
			value = value.Elem()
		}

		switch value.Kind() {
		case reflect.Bool:
			data[field.Name] = strconv.FormatBool(value.Bool())
		case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct, reflect.Interface:
			encoded, err := Marshal(value.Interface())
			if err != nil {
				return nil, fmt.Errorf("failed to marshal Redis hash field %s: %w", field.Name, err)
			}
			data[field.Name] = string(encoded)
		default:
			// 标量保持原有字符串编码，兼容已有缓存。
			data[field.Name] = fmt.Sprintf("%v", value.Interface())
		}
	}
	return data, nil
}

func RedisHSetObj(key string, obj interface{}, expiration time.Duration) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis HSET: key=%s, obj=%+v, expiration=%v", key, obj, expiration))
	}
	ctx := context.Background()
	data, err := redisHashObjectData(obj)
	if err != nil {
		return err
	}

	txn := RDB.TxPipeline()
	txn.HSet(ctx, key, data)

	// 只有在 expiration 大于 0 时才设置过期时间
	if expiration > 0 {
		txn.Expire(ctx, key, expiration)
	}

	_, err = txn.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to execute transaction: %w", err)
	}
	return nil
}

const redisHSetObjIfGenerationScript = `
local current_generation = redis.call("GET", KEYS[1])
if current_generation == false then
    current_generation = "0"
end
if current_generation ~= ARGV[1] then
    return 0
end

local preserve_count = tonumber(ARGV[3])
local preserved_fields = {}
local preserved_values = {}
if redis.call("EXISTS", KEYS[2]) == 1 then
    for index = 1, preserve_count do
        local field = ARGV[3 + index]
        local value = redis.call("HGET", KEYS[2], field)
        if value ~= false then
            table.insert(preserved_fields, field)
            table.insert(preserved_values, value)
        end
    end
end

local field_start = 4 + preserve_count
for index = field_start, #ARGV, 2 do
    redis.call("HSET", KEYS[2], ARGV[index], ARGV[index + 1])
end
for index = 1, #preserved_fields do
    redis.call("HSET", KEYS[2], preserved_fields[index], preserved_values[index])
end

local expiration_ms = tonumber(ARGV[2])
if expiration_ms > 0 then
    redis.call("PEXPIRE", KEYS[2], expiration_ms)
end
return 1
`

func RedisGetGeneration(key string) (int64, error) {
	if RDB == nil {
		return 0, errors.New("redis client is nil")
	}
	generation, err := RDB.Get(context.Background(), key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return generation, err
}

// RedisHSetObjIfGeneration 仅在 generation 未变时回填 hash，并可保留实时字段。
func RedisHSetObjIfGeneration(
	generationKey string,
	dataKey string,
	expectedGeneration int64,
	obj interface{},
	expiration time.Duration,
	preserveFields ...string,
) (bool, error) {
	if DebugEnabled {
		SysLog(fmt.Sprintf(
			"Redis fenced HSET: generation_key=%s, data_key=%s, generation=%d, obj=%+v, expiration=%v, fields=%v",
			generationKey,
			dataKey,
			expectedGeneration,
			obj,
			expiration,
			preserveFields,
		))
	}
	if RDB == nil {
		return false, errors.New("redis client is nil")
	}
	data, err := redisHashObjectData(obj)
	if err != nil {
		return false, err
	}
	arguments := make([]interface{}, 0, 3+len(preserveFields)+len(data)*2)
	arguments = append(arguments, expectedGeneration, expiration.Milliseconds(), len(preserveFields))
	for _, field := range preserveFields {
		arguments = append(arguments, field)
	}
	for field, value := range data {
		arguments = append(arguments, field, value)
	}
	written, err := RDB.Eval(
		context.Background(),
		redisHSetObjIfGenerationScript,
		[]string{generationKey, dataKey},
		arguments...,
	).Int64()
	if err != nil {
		return false, fmt.Errorf("failed to atomically set Redis hash: %w", err)
	}
	return written == 1, nil
}

const redisBumpGenerationAndDeleteKeysScript = `
redis.call("INCR", KEYS[1])
for index = 2, #KEYS do
    redis.call("DEL", KEYS[index])
end
return #KEYS - 1
`

func RedisBumpGenerationAndDeleteKeys(generationKey string, dataKeys []string) error {
	if len(dataKeys) == 0 {
		return nil
	}
	if generationKey == "" {
		return errors.New("redis generation key is empty")
	}
	if RDB == nil {
		return errors.New("redis client is nil")
	}
	keys := make([]string, 1, len(dataKeys)+1)
	keys[0] = generationKey
	keys = append(keys, dataKeys...)
	return RDB.Eval(context.Background(), redisBumpGenerationAndDeleteKeysScript, keys).Err()
}

const redisBumpGenerationAndDeleteIfCurrentScript = `
local current_generation = redis.call("GET", KEYS[1])
if current_generation == false then
    current_generation = "0"
end
if current_generation ~= ARGV[1] then
    return 0
end
redis.call("INCR", KEYS[1])
redis.call("DEL", KEYS[2])
return 1
`

func RedisBumpGenerationAndDeleteIfCurrent(
	generationKey string,
	dataKey string,
	expectedGeneration int64,
) (bool, error) {
	if RDB == nil {
		return false, errors.New("redis client is nil")
	}
	deleted, err := RDB.Eval(
		context.Background(),
		redisBumpGenerationAndDeleteIfCurrentScript,
		[]string{generationKey, dataKey},
		expectedGeneration,
	).Int64()
	if err != nil {
		return false, err
	}
	return deleted == 1, nil
}

func RedisHGetObj(key string, obj interface{}) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis HGETALL: key=%s", key))
	}
	ctx := context.Background()

	result, err := RDB.HGetAll(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to load hash from Redis: %w", err)
	}

	if len(result) == 0 {
		return fmt.Errorf("key %s not found in Redis", key)
	}

	// Handle both pointer and non-pointer values
	val := reflect.ValueOf(obj)
	if val.Kind() != reflect.Ptr {
		return fmt.Errorf("obj must be a pointer to a struct, got %T", obj)
	}

	v := val.Elem()
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("obj must be a pointer to a struct, got pointer to %T", v.Interface())
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldName := field.Name
		if value, ok := result[fieldName]; ok {
			fieldValue := v.Field(i)

			// Handle pointer types
			if fieldValue.Kind() == reflect.Ptr {
				if value == "" {
					continue
				}
				if fieldValue.IsNil() {
					fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
				}
				fieldValue = fieldValue.Elem()
			}

			// Enhanced type handling for cached structs
			switch fieldValue.Kind() {
			case reflect.String:
				fieldValue.SetString(value)
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				intValue, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return fmt.Errorf("failed to parse int field %s: %w", fieldName, err)
				}
				fieldValue.SetInt(intValue)
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				uintValue, err := strconv.ParseUint(value, 10, 64)
				if err != nil {
					return fmt.Errorf("failed to parse uint field %s: %w", fieldName, err)
				}
				fieldValue.SetUint(uintValue)
			case reflect.Float32, reflect.Float64:
				floatValue, err := strconv.ParseFloat(value, fieldValue.Type().Bits())
				if err != nil {
					return fmt.Errorf("failed to parse float field %s: %w", fieldName, err)
				}
				fieldValue.SetFloat(floatValue)
			case reflect.Bool:
				boolValue, err := strconv.ParseBool(value)
				if err != nil {
					return fmt.Errorf("failed to parse bool field %s: %w", fieldName, err)
				}
				fieldValue.SetBool(boolValue)
			case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct, reflect.Interface:
				if err := UnmarshalJsonStr(value, fieldValue.Addr().Interface()); err != nil {
					return fmt.Errorf("failed to unmarshal composite field %s: %w", fieldName, err)
				}
			default:
				return fmt.Errorf("unsupported field type: %s for field %s", fieldValue.Kind(), fieldName)
			}
		}
	}

	return nil
}

// RedisIncr Add this function to handle atomic increments
func RedisIncr(key string, delta int64) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis INCR: key=%s, delta=%d", key, delta))
	}
	// 检查键的剩余生存时间
	ttlCmd := RDB.TTL(context.Background(), key)
	ttl, err := ttlCmd.Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("failed to get TTL: %w", err)
	}

	// 只有在 key 存在且有 TTL 时才需要特殊处理
	if ttl > 0 {
		ctx := context.Background()
		// 开始一个Redis事务
		txn := RDB.TxPipeline()

		// 减少余额
		decrCmd := txn.IncrBy(ctx, key, delta)
		if err := decrCmd.Err(); err != nil {
			return err // 如果减少失败，则直接返回错误
		}

		// 重新设置过期时间，使用原来的过期时间
		txn.Expire(ctx, key, ttl)

		// 执行事务
		_, err = txn.Exec(ctx)
		return err
	}
	return nil
}

func RedisHIncrBy(key, field string, delta int64) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis HINCRBY: key=%s, field=%s, delta=%d", key, field, delta))
	}
	if RDB == nil {
		return errors.New("redis client is nil")
	}
	return RDB.Eval(
		context.Background(),
		redisHashFieldUpdateIfExistsScript,
		[]string{key},
		"HINCRBY",
		field,
		delta,
	).Err()
}

const redisHashFieldUpdateIfExistsScript = `
if redis.call("EXISTS", KEYS[1]) == 0 then
    return 0
end
redis.call(ARGV[1], KEYS[1], ARGV[2], ARGV[3])
return 1
`

func RedisHSetField(key, field string, value interface{}) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis HSET field: key=%s, field=%s, value=%v", key, field, value))
	}
	if RDB == nil {
		return errors.New("redis client is nil")
	}
	return RDB.Eval(
		context.Background(),
		redisHashFieldUpdateIfExistsScript,
		[]string{key},
		"HSET",
		field,
		value,
	).Err()
}
