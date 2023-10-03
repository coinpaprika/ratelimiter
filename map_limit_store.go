package ratelimiter

import (
	"sync"
	"time"
)

// MapLimitStore represents internal limiter data database where data are stored in golang maps
type MapLimitStore struct {
	data           map[int64]map[string]int64
	size           int
	mutex          sync.RWMutex
	expirationTime time.Duration
}

// NewMapLimitStore creates new in-memory data store for internal limiter data. Elements of MapLimitStore is set as expired once beginning of their Window is twice older than expiration time. Expired elements are removed with a period specified by the flushInterval argument
func NewMapLimitStore(expirationTime time.Duration, flushInterval time.Duration) (m *MapLimitStore) {
	m = &MapLimitStore{
		data:           make(map[int64]map[string]int64),
		expirationTime: expirationTime,
	}
	go func() {
		ticker := time.NewTicker(flushInterval)
		for range ticker.C {
			m.mutex.Lock()
			expirationTS := time.Now().Add(-m.expirationTime*2).UnixNano()
			for t, timeSlice := range m.data {
				if t < expirationTS {
					m.size -= len(timeSlice)
					delete(m.data, t)
				}
			}
			m.mutex.Unlock()
		}
	}()
	return m
}

// Inc increments current window limit counter for key
func (m *MapLimitStore) Inc(key string, window time.Time) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	windowTS := window.UnixNano()
	timeSlice, ok := m.data[windowTS]
	if !ok {
		timeSlice := map[string]int64{
			key: 1,
		}
		m.data[windowTS] = timeSlice
		m.size++
	} else {
		oldVal, ok := timeSlice[key]
		if ok {
			timeSlice[key] = oldVal + 1
		} else {
			timeSlice[key] = 1
			m.size++
		}
	}
	return nil
}

func (m *MapLimitStore) lookupCounter(key string, window int64) int64 {
	timeSlice, ok := m.data[window]
	if !ok {
		return 0
	}
	return timeSlice[key]
}

// Get gets value of previous window counter and current window counter for key
func (m *MapLimitStore) Get(key string, previousWindow, currentWindow time.Time) (prevValue int64, currValue int64, err error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	prevValue = m.lookupCounter(key, previousWindow.UnixNano())
	currValue = m.lookupCounter(key, currentWindow.UnixNano())
	return prevValue, currValue, nil
}

// Size returns current length of data map
func (m *MapLimitStore) Size() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.size
}
