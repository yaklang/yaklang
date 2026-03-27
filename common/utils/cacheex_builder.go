package utils

import "time"

func NewCacheExWithTTLAndCapacity[U comparable, T any](ttl time.Duration, capacity int) *CacheExWithKey[U, T] {
	opts := make([]cacheExOption, 0, 2)
	if ttl > 0 {
		opts = append(opts, WithCacheTTL(ttl))
	}
	if capacity > 0 {
		opts = append(opts, WithCacheCapacity(uint64(capacity)))
	}
	return NewCacheExWithKey[U, T](opts...)
}
