package ratelimiter

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisLimitStore represents internal limiter data store where window counters are
// kept in Redis.
type RedisLimitStore struct {
	client     redis.UniversalClient
	expiration time.Duration
	keyPrefix  string
	timeout    time.Duration
	closeOnce  sync.Once
}

// NewRedisLimitStore creates a new Redis-backed data store for internal limiter data.
func NewRedisLimitStore(client redis.UniversalClient, namespace string, expiration time.Duration) *RedisLimitStore {
	return &RedisLimitStore{
		client:     client,
		keyPrefix:  namespace,
		expiration: expiration,
	}
}

// WithTimeout sets a timeout for all Redis commands issued by this store.
// When not set the commands will not have a timeout.
func (r *RedisLimitStore) WithTimeout(timeout time.Duration) *RedisLimitStore {
	r.timeout = timeout
	return r
}

// Inc increments current window limit counter for key and refreshes its TTL
func (r *RedisLimitStore) Inc(key string, window time.Time) error {
	ctx := context.Background()
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	k := r.redisKey(key, window)
	pipe := r.client.Pipeline()
	pipe.Incr(ctx, k)
	pipe.Expire(ctx, k, r.expiration)
	_, err := pipe.Exec(ctx)
	return err
}

// Get gets value of previous window counter and current window counter for key.
func (r *RedisLimitStore) Get(
	key string,
	previousWindow, currentWindow time.Time,
) (prevValue int64, currValue int64, err error) {
	ctx := context.Background()
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	pipe := r.client.Pipeline()
	prevCmd := pipe.Get(ctx, r.redisKey(key, previousWindow))
	currCmd := pipe.Get(ctx, r.redisKey(key, currentWindow))

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return 0, 0, err
	}

	if prevValue, err = counterValue(prevCmd); err != nil {
		return 0, 0, err
	}
	if currValue, err = counterValue(currCmd); err != nil {
		return 0, 0, err
	}
	return prevValue, currValue, nil
}

func (r *RedisLimitStore) Close() error {
	var err error
	r.closeOnce.Do(func() {
		err = r.client.Close()
	})
	return err
}

// counterValue reads an int64 counter from a GET command, treating a missing key as zero
func counterValue(cmd *redis.StringCmd) (int64, error) {
	v, err := cmd.Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (r *RedisLimitStore) redisKey(key string, window time.Time) string {
	return r.keyPrefix + key + "_" + window.Format(time.RFC3339)
}
