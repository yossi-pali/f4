package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/12go/f4/internal/config"
)

// RedisCache implements Cache using Redis.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis cache client.
func NewRedisCache(cfg config.CacheConfig) *RedisCache {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	return &RedisCache{client: client}
}

func (c *RedisCache) Get(ctx context.Context, key string, dest any) error {
	val, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return ErrCacheMiss{}
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(val, dest)
}

func (c *RedisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

// Close closes the Redis connection.
func (c *RedisCache) Close() error {
	return c.client.Close()
}
