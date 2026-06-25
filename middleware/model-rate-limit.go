package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
)

type modelRequestRateLimitRule struct {
	name            string
	scope           string
	totalMaxCount   int
	successMaxCount int
}

var errModelRequestRateLimited = errors.New("model_request_rate_limited")
var errModelRequestRateLimitCheckFailed = errors.New("model_request_rate_limit_check_failed")

// 检查Redis中的请求限制
func checkRedisRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) (bool, error) {
	// 如果maxCount为0，表示不限制
	if maxCount == 0 {
		return true, nil
	}

	// 获取当前计数
	length, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		return false, err
	}

	// 如果未达到限制，允许请求
	if length < int64(maxCount) {
		return true, nil
	}

	// 检查时间窗口
	oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
	oldTime, err := time.Parse(timeFormat, oldTimeStr)
	if err != nil {
		return false, err
	}

	nowTimeStr := time.Now().Format(timeFormat)
	nowTime, err := time.Parse(timeFormat, nowTimeStr)
	if err != nil {
		return false, err
	}
	// 如果在时间窗口内已达到限制，拒绝请求
	subTime := nowTime.Sub(oldTime).Seconds()
	if int64(subTime) < duration {
		rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
		return false, nil
	}

	return true, nil
}

// 记录Redis请求
func recordRedisRequest(ctx context.Context, rdb *redis.Client, key string, maxCount int) {
	// 如果maxCount为0，不记录请求
	if maxCount == 0 {
		return
	}

	now := time.Now().Format(timeFormat)
	rdb.LPush(ctx, key, now)
	rdb.LTrim(ctx, key, 0, int64(maxCount-1))
	rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
}

func buildModelRequestRateLimitRule(userGroup, group string) modelRequestRateLimitRule {
	if userGroupTotalCount, userGroupSuccessCount, found := setting.GetUserGroupRateLimit(userGroup, group); found {
		return modelRequestRateLimitRule{
			name:            "user_group_request_group",
			scope:           fmt.Sprintf("user_group:%s:request_group:%s", userGroup, group),
			totalMaxCount:   userGroupTotalCount,
			successMaxCount: userGroupSuccessCount,
		}
	}

	if userTotalCount, userSuccessCount, found := setting.GetUserGroupGlobalRateLimit(userGroup); found {
		return modelRequestRateLimitRule{
			name:            "user_group",
			scope:           fmt.Sprintf("user_group:%s", userGroup),
			totalMaxCount:   userTotalCount,
			successMaxCount: userSuccessCount,
		}
	}

	if groupTotalCount, groupSuccessCount, found := setting.GetGroupRateLimit(group); found {
		return modelRequestRateLimitRule{
			name:            "request_group",
			scope:           fmt.Sprintf("request_group:%s", group),
			totalMaxCount:   groupTotalCount,
			successMaxCount: groupSuccessCount,
		}
	}

	return modelRequestRateLimitRule{
		name:            "global",
		scope:           "global",
		totalMaxCount:   setting.ModelRequestRateLimitCount,
		successMaxCount: setting.ModelRequestRateLimitSuccessCount,
	}
}

func escapeModelRequestRateLimitScope(scope string) string {
	return url.QueryEscape(scope)
}

func buildModelRequestRateLimitKey(limitType, scope, userId string) string {
	return fmt.Sprintf("rateLimit:model_request:v2:%s:%s:user:%s", limitType, escapeModelRequestRateLimitScope(scope), userId)
}

func buildModelRequestRateLimitMemoryKey(limitType, scope, userId string) string {
	return fmt.Sprintf("%s:v2:%s:%s:user:%s", ModelRequestRateLimitCountMark, limitType, escapeModelRequestRateLimitScope(scope), userId)
}

// Redis限流处理器
func redisRateLimitHandler(duration int64, rule modelRequestRateLimitRule) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		ctx := context.Background()
		rdb := common.RDB

		allowed, err := checkRedisModelRequestRateLimit(ctx, rdb, userId, duration, rule)
		if err != nil {
			fmt.Println("检查模型请求限流失败:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			abortModelRequestRateLimit(c, rule)
			return
		}

		setupSelectedChannelAfterModelRequestRateLimit(c)
		if c.IsAborted() {
			return
		}
		c.Next()

		if c.Writer.Status() < 400 {
			successRule := currentModelRequestRateLimitRule(c)
			successKey := buildModelRequestRateLimitKey("success", successRule.scope, userId)
			recordRedisRequest(ctx, rdb, successKey, successRule.successMaxCount)
		}
	}
}

func checkRedisModelRequestRateLimit(ctx context.Context, rdb *redis.Client, userId string, duration int64, rule modelRequestRateLimitRule) (bool, error) {
	successKey := buildModelRequestRateLimitKey("success", rule.scope, userId)
	allowed, err := checkRedisRateLimit(ctx, rdb, successKey, rule.successMaxCount, duration)
	if err != nil || !allowed {
		return allowed, err
	}

	if rule.totalMaxCount == 0 {
		return true, nil
	}
	totalKey := buildModelRequestRateLimitKey("total", rule.scope, userId)
	tb := limiter.New(ctx, rdb)
	return tb.Allow(
		ctx,
		totalKey,
		limiter.WithCapacity(int64(rule.totalMaxCount)*duration),
		limiter.WithRate(int64(rule.totalMaxCount)),
		limiter.WithRequested(duration),
	)
}

// 内存限流处理器
func memoryRateLimitHandler(duration int64, rule modelRequestRateLimitRule) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)

	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))

		if !checkMemoryModelRequestRateLimit(userId, duration, rule) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		setupSelectedChannelAfterModelRequestRateLimit(c)
		if c.IsAborted() {
			return
		}
		c.Next()

		if c.Writer.Status() < 400 {
			successRule := currentModelRequestRateLimitRule(c)
			successKey := buildModelRequestRateLimitMemoryKey("success", successRule.scope, userId)
			inMemoryRateLimiter.Request(successKey, successRule.successMaxCount, duration)
		}
	}
}

func checkMemoryModelRequestRateLimit(userId string, duration int64, rule modelRequestRateLimitRule) bool {
	totalKey := buildModelRequestRateLimitMemoryKey("total", rule.scope, userId)
	if rule.totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, rule.totalMaxCount, duration) {
		return false
	}
	successKey := buildModelRequestRateLimitMemoryKey("success", rule.scope, userId)
	return inMemoryRateLimiter.Allow(successKey, rule.successMaxCount, duration)
}

func currentModelRequestRateLimitGroup(c *gin.Context) string {
	group := common.GetContextKeyString(c, constant.ContextKeySelectedChannelGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	}
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	}
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	return group
}

func currentModelRequestRateLimitRule(c *gin.Context) modelRequestRateLimitRule {
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	group := currentModelRequestRateLimitGroup(c)
	return buildModelRequestRateLimitRule(userGroup, group)
}

func CheckModelRequestRateLimitForGroup(c *gin.Context, group string) *types.NewAPIError {
	if !setting.ModelRequestRateLimitEnabled {
		return nil
	}
	userId := strconv.Itoa(c.GetInt("id"))
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	rule := buildModelRequestRateLimitRule(userGroup, group)
	duration := int64(setting.ModelRequestRateLimitDurationMinutes * 60)

	if common.RedisEnabled {
		allowed, err := checkRedisModelRequestRateLimit(context.Background(), common.RDB, userId, duration, rule)
		if err != nil {
			return types.NewErrorWithStatusCode(errModelRequestRateLimitCheckFailed, types.ErrorCodeInvalidRequest, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
		}
		if !allowed {
			return types.NewErrorWithStatusCode(errModelRequestRateLimited, types.ErrorCodeInvalidRequest, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())
		}
		return nil
	}

	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)
	if !checkMemoryModelRequestRateLimit(userId, duration, rule) {
		return types.NewErrorWithStatusCode(errModelRequestRateLimited, types.ErrorCodeInvalidRequest, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())
	}
	return nil
}

func abortModelRequestRateLimit(c *gin.Context, rule modelRequestRateLimitRule) {
	abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, rule.successMaxCount))
}

func setupSelectedChannelAfterModelRequestRateLimit(c *gin.Context) {
	selectedChannel, ok := common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel)
	if !ok {
		return
	}
	modelName := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	if err := SetupContextForSelectedChannel(c, selectedChannel, modelName); err != nil {
		abortWithOpenAiMessage(c, err.StatusCode, err.Error(), err.GetErrorCode())
	}
}

func shouldSkipModelRequestRateLimit(c *gin.Context) bool {
	if _, ok := common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel); ok {
		return false
	}
	return common.GetContextKeyString(c, constant.ContextKeyOriginalModel) == ""
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 在每个请求时检查是否启用限流
		if !setting.ModelRequestRateLimitEnabled || shouldSkipModelRequestRateLimit(c) {
			setupSelectedChannelAfterModelRequestRateLimit(c)
			if c.IsAborted() {
				return
			}
			c.Next()
			return
		}

		duration := int64(setting.ModelRequestRateLimitDurationMinutes * 60)

		rule := currentModelRequestRateLimitRule(c)

		// 根据存储类型选择并执行限流处理器
		if common.RedisEnabled {
			redisRateLimitHandler(duration, rule)(c)
		} else {
			memoryRateLimitHandler(duration, rule)(c)
		}
	}
}
