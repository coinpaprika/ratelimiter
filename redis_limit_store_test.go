package ratelimiter

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRedisStore spins up an in-process miniredis and returns a store wired to it
// together with the miniredis handle for time/TTL assertions.
func newTestRedisStore(
	t *testing.T,
	namespace string,
	expiration time.Duration,
) (*RedisLimitStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t, client.Ping(t.Context()).Err())
	store := NewRedisLimitStore(client, namespace, expiration)
	t.Cleanup(func() { _ = store.Close() })
	return store, mr
}

func TestNewRedisLimitStore(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t, client.Ping(t.Context()).Err())

	ns := "ratelimiter:"
	store := NewRedisLimitStore(client, ns, 1*time.Minute)
	assert.Equal(t, 1*time.Minute, store.expiration)
	assert.Equal(t, ns, store.keyPrefix)
	assert.Zero(t, store.timeout)
}

func TestRedisLimitStore_WithTimeout(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t, client.Ping(t.Context()).Err())

	store := NewRedisLimitStore(client, "ratelimiter:", 1*time.Minute).WithTimeout(5 * time.Second)
	t.Cleanup(func() { _ = store.Close() })

	assert.Equal(t, 5*time.Second, store.timeout)

	window := time.Now().UTC()
	require.NoError(t, store.Inc("tt", window))

	prevVal, currVal, err := store.Get("tt", window, window)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), prevVal)
	assert.Equal(t, int64(1), currVal)
}

func TestRedisLimitStore_WithTimeout_Exceeded(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t, client.Ping(t.Context()).Err())

	store := NewRedisLimitStore(client, "ratelimiter:", 1*time.Minute).WithTimeout(1 * time.Nanosecond)
	t.Cleanup(func() { _ = store.Close() })

	window := time.Now().UTC()
	err := store.Inc("tt", window)
	assert.Error(t, err)

	_, _, err = store.Get("tt", window, window)
	assert.Error(t, err)
}

func TestRedisLimitStore_Inc(t *testing.T) {
	store, mr := newTestRedisStore(t, "ratelimiter:", 1*time.Minute)
	window := time.Now().UTC()

	require.NoError(t, store.Inc("tt", window))

	prevVal, currVal, err := store.Get("tt", window, window)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), prevVal)
	assert.Equal(t, int64(1), currVal)

	assert.Positive(t, mr.TTL(store.redisKey("tt", window)))

	require.NoError(t, store.Inc("tt", window))
	_, currVal, err = store.Get("tt", window, window)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), currVal)
}

func TestRedisLimitStore_Get(t *testing.T) {
	store, _ := newTestRedisStore(t, "ratelimiter:", 1*time.Minute)
	previousWindow := time.Now().UTC().Add(-1 * time.Minute)
	currentWindow := time.Now().UTC()

	for range 10 {
		require.NoError(t, store.Inc("tt", previousWindow))
	}
	for range 5 {
		require.NoError(t, store.Inc("tt", currentWindow))
	}

	prevVal, currVal, err := store.Get("tt", previousWindow, currentWindow)
	assert.NoError(t, err)
	assert.Equal(t, int64(10), prevVal)
	assert.Equal(t, int64(5), currVal)

	prevVal, currVal, err = store.Get("missing", previousWindow, currentWindow)
	assert.NoError(t, err)
	assert.Zero(t, prevVal)
	assert.Zero(t, currVal)
}

func TestRedisLimitStore_Expiry(t *testing.T) {
	store, mr := newTestRedisStore(t, "ratelimiter:", 1*time.Minute)
	window := time.Now().UTC()

	require.NoError(t, store.Inc("tt", window))
	_, currVal, err := store.Get("tt", window, window)
	require.NoError(t, err)
	require.Equal(t, int64(1), currVal)

	mr.FastForward(2 * time.Minute)

	_, currVal, err = store.Get("tt", window, window)
	assert.NoError(t, err)
	assert.Zero(t, currVal)
}

func TestRedisLimitStore_KeyPrefix(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t, client.Ping(t.Context()).Err())

	storeA := NewRedisLimitStore(client, "a:", 1*time.Minute)
	storeB := NewRedisLimitStore(client, "b:", 1*time.Minute)
	window := time.Now().UTC()

	require.NoError(t, storeA.Inc("tt", window))

	_, currA, err := storeA.Get("tt", window, window)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), currA)

	_, currB, err := storeB.Get("tt", window, window)
	assert.NoError(t, err)
	assert.Zero(t, currB)
}

func TestRedisLimitStore_Close(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t, client.Ping(t.Context()).Err())
	store := NewRedisLimitStore(client, "ratelimiter:", 1*time.Minute)

	assert.NotPanics(t, func() {
		_ = store.Close()
		_ = store.Close()
	})
}

func TestRedisLimitStore_RateLimiter(t *testing.T) {
	store, _ := newTestRedisStore(t, "ratelimiter:", 2*time.Minute)
	limiter := New(store, 5, 1*time.Minute)
	key := "e2e"

	for range 5 {
		status, err := limiter.Check(key)
		require.NoError(t, err)
		require.False(t, status.IsLimited)
		require.NoError(t, limiter.Inc(key))
	}

	status, err := limiter.Check(key)
	require.NoError(t, err)
	assert.True(t, status.IsLimited)
	assert.NotNil(t, status.LimitDuration)
}

func TestRedisLimitStore_Pipeline_IncError(t *testing.T) {
	store, mr := newTestRedisStore(t, "ratelimiter:", 1*time.Minute)
	window := time.Now().UTC()

	key := store.redisKey("bad_key", window)
	require.NoError(t, mr.Set(key, "not_a_number"))

	err := store.Inc("bad_key", window)
	assert.Error(t, err)
}

func TestRedisLimitStore_Pipeline_GetOneKeyMissing(t *testing.T) {
	store, _ := newTestRedisStore(t, "ratelimiter:", 1*time.Minute)
	previousWindow := time.Now().UTC().Add(-1 * time.Minute)
	currentWindow := time.Now().UTC()

	require.NoError(t, store.Inc("tt", previousWindow))

	prevVal, currVal, err := store.Get("tt", previousWindow, currentWindow)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), prevVal)
	assert.Equal(t, int64(0), currVal)
}

func TestRedisLimitStore_Pipeline_GetOneKeyInvalidValue(t *testing.T) {
	store, mr := newTestRedisStore(t, "ratelimiter:", 1*time.Minute)
	previousWindow := time.Now().UTC().Add(-1 * time.Minute)
	currentWindow := time.Now().UTC()

	require.NoError(t, store.Inc("tt", previousWindow))
	currKey := store.redisKey("tt", currentWindow)
	require.NoError(t, mr.Set(currKey, "corrupted_val"))

	prevVal, currVal, err := store.Get("tt", previousWindow, currentWindow)
	assert.Error(t, err)
	assert.Zero(t, prevVal)
	assert.Zero(t, currVal)
}

func TestRedisLimitStore_Pipeline_GetWrongType(t *testing.T) {
	store, mr := newTestRedisStore(t, "ratelimiter:", 1*time.Minute)
	previousWindow := time.Now().UTC().Add(-1 * time.Minute)
	currentWindow := time.Now().UTC()

	prevKey := store.redisKey("tt", previousWindow)
	mr.HSet(prevKey, "field", "value")

	require.NoError(t, store.Inc("tt", currentWindow))

	prevVal, currVal, err := store.Get("tt", previousWindow, currentWindow)
	assert.Error(t, err)
	assert.Zero(t, prevVal)
	assert.Zero(t, currVal)
}
