package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dujiao-next/internal/config"

	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client
var redisPrefix string
var redisEnabled bool

// InitRedis 初始化 Redis 客户端
func InitRedis(cfg *config.RedisConfig) error {
	if cfg == nil || !cfg.Enabled {
		redisEnabled = false
		return nil
	}
	addr := strings.TrimSpace(cfg.Host)
	if addr == "" {
		addr = "127.0.0.1"
	}
	port := cfg.Port
	if port <= 0 {
		port = 6379
	}
	redisPrefix = strings.TrimSpace(cfg.Prefix)
	if redisPrefix == "" {
		redisPrefix = "dj"
	}

	redisClient = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", addr, port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	redisEnabled = true
	return nil
}

// Enabled 判断缓存是否启用
func Enabled() bool {
	return redisEnabled && redisClient != nil
}

// Client 获取 Redis 客户端
func Client() *redis.Client {
	if !Enabled() {
		return nil
	}
	return redisClient
}

// GetJSON 获取 JSON 缓存
func GetJSON(ctx context.Context, key string, dest interface{}) (bool, error) {
	if !Enabled() {
		return false, nil
	}
	val, err := redisClient.Get(ctx, buildKey(key)).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(val), dest); err != nil {
		return false, err
	}
	return true, nil
}

// SetJSON 写入 JSON 缓存
func SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if !Enabled() {
		return nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return redisClient.Set(ctx, buildKey(key), payload, ttl).Err()
}

// Del 删除缓存
func Del(ctx context.Context, key string) error {
	if !Enabled() {
		return nil
	}
	return redisClient.Del(ctx, buildKey(key)).Err()
}

func buildKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return redisPrefix
	}
	return fmt.Sprintf("%s:%s", redisPrefix, trimmed)
}
