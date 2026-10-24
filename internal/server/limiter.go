package server

import (
	"sync"
	"time"
)

// limiter đếm số lần (ví dụ đăng nhập sai) theo khoá trong một cửa sổ thời gian trượt.
type limiter struct {
	max    int
	window time.Duration
	now    func() time.Time

	mu   sync.Mutex
	hits map[string][]time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, now: time.Now, hits: map[string][]time.Time{}}
}

// recent bỏ các lần đã ra khỏi cửa sổ; gọi khi đang giữ mu.
func (l *limiter) recent(key string) []time.Time {
	cutoff := l.now().Add(-l.window)
	hits := l.hits[key]
	i := 0
	for i < len(hits) && hits[i].Before(cutoff) {
		i++
	}
	hits = hits[i:]
	if len(hits) == 0 {
		delete(l.hits, key)
	} else {
		l.hits[key] = hits
	}
	return hits
}

func (l *limiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(key)) >= l.max
}

func (l *limiter) add(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[key] = append(l.recent(key), l.now())
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}
