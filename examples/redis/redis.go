package main

import (
	"fmt"
	"log"
	"time"

	"github.com/coinpaprika/ratelimiter"
	"github.com/redis/go-redis/v9"
)

func main() {
	limitedKey := "key"
	windowSize := 1 * time.Minute

	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

	dataStore := ratelimiter.NewRedisLimitStore(
		client,
		"ratelimiter:",
		2*windowSize,
	) // TTL per window counter; keep at least 2*windowSize
	defer func() { _ = dataStore.Close() }() // closes the Redis client

	var maxLimit int64 = 5
	rateLimiter := ratelimiter.New(dataStore, maxLimit, windowSize) // allow 5 requests per windowSize (1 minute)

	for range 10 {
		limitStatus, err := rateLimiter.Check(limitedKey)
		if err != nil {
			log.Fatal(err)
		}
		if limitStatus.IsLimited {
			fmt.Printf(
				"too high rate for key: %s: rate: %f, limit: %d\nsleep: %s",
				limitedKey,
				limitStatus.CurrentRate,
				maxLimit,
				*limitStatus.LimitDuration,
			)
			time.Sleep(*limitStatus.LimitDuration)
		} else {
			err := rateLimiter.Inc(limitedKey)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}
