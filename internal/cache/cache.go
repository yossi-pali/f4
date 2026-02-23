package cache

import (
	"context"
	"time"
)

// Cache is the interface for key-value caching.
type Cache interface {
	Get(ctx context.Context, key string, dest any) error
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// ErrCacheMiss is returned when a key is not found.
type ErrCacheMiss struct{}

func (e ErrCacheMiss) Error() string { return "cache miss" }

// IsCacheMiss checks if the error is a cache miss.
func IsCacheMiss(err error) bool {
	_, ok := err.(ErrCacheMiss)
	return ok
}
