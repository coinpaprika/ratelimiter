package ratelimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewMapLimitStore(t *testing.T) {

	type args struct {
		expirationTime time.Duration
		flushInterval  time.Duration
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "test_NewMapLimitStore",
			args: args{
				expirationTime: 1 * time.Minute,
				flushInterval:  2 * time.Second,
			},
		},
	}
	for _, tt := range tests {
		mapLimitStore := NewMapLimitStore(tt.args.expirationTime, tt.args.flushInterval)
		assert.Equal(t, tt.args.expirationTime, mapLimitStore.expirationTime)
	}
}

func TestMapLimitStore_Inc(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		window  time.Time
		wantErr bool
	}{
		{
			name:    "test_MapLimitStore_Inc",
			key:     "tt",
			window:  time.Now().UTC(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		m := NewMapLimitStore(1*time.Minute, 10*time.Second)
		err := m.Inc(tt.key, tt.window)
		assert.NoError(t, err)
		prevVal, currVal, err := m.Get(tt.key, tt.window, tt.window)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), prevVal)
		assert.Equal(t, int64(1), currVal)
	}
}

func TestMapLimitStore_Get(t *testing.T) {
	type args struct {
		key            string
		previousWindow time.Time
		currentWindow  time.Time
	}
	tests := []struct {
		name          string
		args          args
		wantPrevValue int64
		wantCurrValue int64
		wantErr       bool
	}{
		{
			name: "test_MapLimitStore_Get",
			args: args{
				key:            "tt",
				previousWindow: time.Now().UTC().Add(-1 * time.Minute),
				currentWindow:  time.Now().UTC(),
			},
			wantPrevValue: 10,
			wantCurrValue: 5,
			wantErr:       false,
		},
	}
	for _, tt := range tests {
		m := NewMapLimitStore(1*time.Minute, 10*time.Second)
		m.data[mapKey(tt.args.key, tt.args.previousWindow)] = limitValue{val: tt.wantPrevValue}
		m.data[mapKey(tt.args.key, tt.args.currentWindow)] = limitValue{val: tt.wantCurrValue}

		prevVal, currVal, err := m.Get(tt.args.key, tt.args.previousWindow, tt.args.currentWindow)
		assert.NoError(t, err)
		assert.Equal(t, tt.wantPrevValue, prevVal)
		assert.Equal(t, tt.wantCurrValue, currVal)
	}
}

func TestMapLimitStore_flushExpired(t *testing.T) {
	m := NewMapLimitStore(50*time.Millisecond, 1*time.Hour)
	defer m.Close()

	now := time.Now().UTC()
m.mutex.Lock()
m.data[mapKey("fresh", now)] = limitValue{val: 1, lastUpdate: now}
m.data[mapKey("stale", now)] = limitValue{val: 1, lastUpdate: now.Add(-1 * time.Hour)}
m.mutex.Unlock()
	m.flushExpired()

	assert.Equal(t, 1, m.Size())
	prevVal, _, err := m.Get("fresh", now, now)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), prevVal)
}

func TestMapLimitStore_Close(t *testing.T) {
	m := NewMapLimitStore(1*time.Minute, 10*time.Millisecond)
	// Close stops the background goroutine and must be safe to call multiple times.
	assert.NotPanics(t, func() {
		m.Close()
		m.Close()
	})
}

func TestMapLimitStore_Size(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		window time.Time
		size   int
	}{
		{
			name:   "test_MapLimitStore_Size",
			key:    "tt",
			window: time.Now().UTC(),
			size:   1,
		},
		{
			name:   "test_MapLimitStore_Size",
			key:    "tt",
			window: time.Time{},
			size:   0,
		},
	}
	for _, tt := range tests {
		m := NewMapLimitStore(1*time.Minute, 10*time.Second)
		if !tt.window.IsZero() {
			err := m.Inc(tt.key, tt.window)
			assert.NoError(t, err)
		}
		assert.Equal(t, tt.size, m.Size())
	}
}
