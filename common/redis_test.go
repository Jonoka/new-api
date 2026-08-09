package common_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

type redisHashRoundTripServer struct {
	mu          sync.Mutex
	hashes      map[string]map[string]string
	generations map[string]int64
}

func newRedisHashRoundTripClient(server *redisHashRoundTripServer) *redis.Client {
	return redis.NewClient(&redis.Options{
		MaxRetries: -1,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			clientConn, serverConn := net.Pipe()
			go server.serve(serverConn)
			return clientConn, nil
		},
	})
}

func readRedisCommand(reader *bufio.Reader) ([]string, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	count, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(header), "*"))
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, count)
	for range count {
		lengthHeader, readErr := reader.ReadString('\n')
		if readErr != nil {
			return nil, readErr
		}
		length, parseErr := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(lengthHeader), "$"))
		if parseErr != nil || length < 0 {
			return nil, fmt.Errorf("invalid Redis bulk string length %q", lengthHeader)
		}
		payload := make([]byte, length+2)
		if _, readErr = io.ReadFull(reader, payload); readErr != nil {
			return nil, readErr
		}
		args = append(args, string(payload[:length]))
	}
	return args, nil
}

func writeRedisSimple(conn net.Conn, value string) error {
	_, err := io.WriteString(conn, "+"+value+"\r\n")
	return err
}

func writeRedisInteger(conn net.Conn, value int) error {
	_, err := fmt.Fprintf(conn, ":%d\r\n", value)
	return err
}

func (server *redisHashRoundTripServer) executeIntegerCommand(args []string) (int, error) {
	if len(args) == 0 {
		return 0, fmt.Errorf("empty Redis command")
	}
	switch strings.ToUpper(args[0]) {
	case "HSET":
		if len(args) < 4 || len(args)%2 != 0 {
			return 0, fmt.Errorf("invalid HSET arguments")
		}
		server.mu.Lock()
		defer server.mu.Unlock()
		fields := server.hashes[args[1]]
		if fields == nil {
			fields = make(map[string]string)
			server.hashes[args[1]] = fields
		}
		added := 0
		for index := 2; index < len(args); index += 2 {
			if _, exists := fields[args[index]]; !exists {
				added++
			}
			fields[args[index]] = args[index+1]
		}
		return added, nil
	case "EXPIRE", "PEXPIRE":
		return 1, nil
	case "EVAL":
		if len(args) < 4 {
			return 0, fmt.Errorf("invalid EVAL arguments")
		}
		keyCount, err := strconv.Atoi(args[2])
		if err != nil || keyCount < 1 || len(args) < 3+keyCount {
			return 0, fmt.Errorf("invalid EVAL key count")
		}
		server.mu.Lock()
		defer server.mu.Unlock()
		if server.generations == nil {
			server.generations = make(map[string]int64)
		}
		server.generations[args[3]]++
		for _, key := range args[4 : 3+keyCount] {
			delete(server.hashes, key)
		}
		return keyCount - 1, nil
	default:
		return 0, fmt.Errorf("unsupported Redis command %q", args[0])
	}
}

func (server *redisHashRoundTripServer) writeHash(conn net.Conn, key string) error {
	server.mu.Lock()
	fields := make(map[string]string, len(server.hashes[key]))
	for field, value := range server.hashes[key] {
		fields[field] = value
	}
	server.mu.Unlock()

	names := make([]string, 0, len(fields))
	for field := range fields {
		names = append(names, field)
	}
	sort.Strings(names)
	if _, err := fmt.Fprintf(conn, "*%d\r\n", len(names)*2); err != nil {
		return err
	}
	for _, field := range names {
		value := fields[field]
		if _, err := fmt.Fprintf(conn, "$%d\r\n%s\r\n$%d\r\n%s\r\n", len(field), field, len(value), value); err != nil {
			return err
		}
	}
	return nil
}

func (server *redisHashRoundTripServer) serve(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	var queued [][]string
	for {
		args, err := readRedisCommand(reader)
		if err != nil {
			return
		}
		command := strings.ToUpper(args[0])
		switch {
		case command == "MULTI":
			queued = make([][]string, 0)
			if writeRedisSimple(conn, "OK") != nil {
				return
			}
		case command == "EXEC":
			if _, err = fmt.Fprintf(conn, "*%d\r\n", len(queued)); err != nil {
				return
			}
			for _, queuedArgs := range queued {
				value, executeErr := server.executeIntegerCommand(queuedArgs)
				if executeErr != nil {
					_, _ = fmt.Fprintf(conn, "-%s\r\n", executeErr.Error())
					continue
				}
				if writeRedisInteger(conn, value) != nil {
					return
				}
			}
			queued = nil
		case queued != nil:
			queued = append(queued, args)
			if writeRedisSimple(conn, "QUEUED") != nil {
				return
			}
		case command == "HGETALL" && len(args) == 2:
			if server.writeHash(conn, args[1]) != nil {
				return
			}
		case command == "PING":
			if writeRedisSimple(conn, "PONG") != nil {
				return
			}
		default:
			value, executeErr := server.executeIntegerCommand(args)
			if executeErr != nil {
				_, _ = fmt.Fprintf(conn, "-%s\r\n", executeErr.Error())
				continue
			}
			if writeRedisInteger(conn, value) != nil {
				return
			}
		}
	}
}

func TestRedisHashTokenCompositeFieldsRoundTrip(t *testing.T) {
	server := &redisHashRoundTripServer{hashes: make(map[string]map[string]string)}
	client := newRedisHashRoundTripClient(server)
	t.Cleanup(func() { _ = client.Close() })

	previousClient := common.RDB
	common.RDB = client
	t.Cleanup(func() { common.RDB = previousClient })

	allowedIPs := "127.0.0.1\n10.0.0.1"
	original := model.Token{
		Id:             19,
		UserId:         7,
		Status:         1,
		Name:           "round-trip-token",
		RemainQuota:    987654,
		UnlimitedQuota: false,
		AllowIps:       &allowedIPs,
		Group:          "group-a,group-b",
		GroupMode:      model.TokenGroupModeExplicit,
		GroupIds:       []int{341, 512},
		GroupDetails: []model.GroupReference{
			{Id: 341, Code: "group-a", Name: "分组 A"},
			{Id: 512, Code: "group-b", Name: "分组 B"},
		},
	}

	require.NoError(t, common.RedisHSetObj("token:round-trip", &original, time.Minute))

	var restored model.Token
	require.NoError(t, common.RedisHGetObj("token:round-trip", &restored))
	require.Equal(t, original.GroupIds, restored.GroupIds)
	require.Equal(t, original.GroupDetails, restored.GroupDetails)
	require.Equal(t, original.RemainQuota, restored.RemainQuota)
	require.Equal(t, original.AllowIps, restored.AllowIps)
	require.Equal(t, original.Name, restored.Name)
	require.Equal(t, original.GroupMode, restored.GroupMode)
}

type redisCompositeSnapshot struct {
	Ratios  map[string]float64
	Details struct {
		Enabled bool
		Name    string
	}
}

func TestRedisHashMapAndStructFieldsRoundTrip(t *testing.T) {
	server := &redisHashRoundTripServer{hashes: make(map[string]map[string]string)}
	client := newRedisHashRoundTripClient(server)
	t.Cleanup(func() { _ = client.Close() })

	previousClient := common.RDB
	common.RDB = client
	t.Cleanup(func() { common.RDB = previousClient })

	original := redisCompositeSnapshot{Ratios: map[string]float64{"default": 1, "vip": 0.2}}
	original.Details.Enabled = true
	original.Details.Name = "复合字段"

	require.NoError(t, common.RedisHSetObj("composite:round-trip", &original, time.Minute))

	var restored redisCompositeSnapshot
	require.NoError(t, common.RedisHGetObj("composite:round-trip", &restored))
	require.Equal(t, original, restored)
}

func TestRedisBatchInvalidationBumpsOneGlobalGeneration(t *testing.T) {
	server := &redisHashRoundTripServer{
		hashes: map[string]map[string]string{
			"token:a": {"RemainQuota": "100"},
			"token:b": {"RemainQuota": "200"},
		},
	}
	client := newRedisHashRoundTripClient(server)
	t.Cleanup(func() { _ = client.Close() })

	previousClient := common.RDB
	common.RDB = client
	t.Cleanup(func() { common.RDB = previousClient })

	require.NoError(t, common.RedisBumpGenerationAndDeleteKeys(
		"token-cache-generation",
		[]string{"token:a", "token:b"},
	))

	server.mu.Lock()
	defer server.mu.Unlock()
	require.EqualValues(t, 1, server.generations["token-cache-generation"])
	require.NotContains(t, server.hashes, "token:a")
	require.NotContains(t, server.hashes, "token:b")
}
