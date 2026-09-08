package upload

import (
	"sync"
	"time"
)

// rateLimiter 是按客户端 IP 的滑动窗口计数限速器，固定容量、自动过期，
// 无第三方依赖。Zero 值不可用，须用 newRateLimiter 创建。
type rateLimiter struct {
	mu       sync.Mutex
	perMin   int           // 每 allowWindow 允许的请求数
	window   time.Duration // 统计窗口
	maxKeys  int           // key 数上限，超过时淘汰最旧的，防内存被伪造 IP 撑爆
	entries  map[string]*bucket
	evictSeq uint64 // 访问序号，用于近似 LRU 淘汰
}

// bucket 记录一个 IP 的窗口内请求时间戳，保持时间升序（times[0] 最早）。
// 窗口清理通过前移压缩，切片长度不超过 perMin，内存有界。
type bucket struct {
	times []time.Time
	seq   uint64
}

// newRateLimiter 创建限速器。perMin <= 0 表示不限速，Allow 恒真且不记录。
func newRateLimiter(perMin int, window time.Duration) *rateLimiter {
	if window <= 0 {
		window = time.Minute
	}
	return &rateLimiter{
		perMin:  perMin,
		window:  window,
		maxKeys: 65536,
		entries: make(map[string]*bucket),
	}
}

// Allow 报告 key 在当前窗口内是否还有配额，并有条件地记录本次请求。
func (rl *rateLimiter) Allow(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.perMin <= 0 {
		return true // 不限速模式不记录，entries 不增长
	}

	b, ok := rl.entries[key]
	if !ok {
		if len(rl.entries) >= rl.maxKeys {
			rl.evictOldest()
		}
		b = &bucket{}
		rl.entries[key] = b
	}
	rl.evictSeq++
	b.seq = rl.evictSeq

	// 丢弃窗口外的最旧记录（升序，从头部扫描）。
	cutoff := now.Add(-rl.window)
	drop := 0
	for drop < len(b.times) && b.times[drop].Before(cutoff) {
		drop++
	}
	if drop > 0 {
		b.times = append(b.times[:0], b.times[drop:]...)
	}
	if len(b.times) >= rl.perMin {
		return false
	}
	b.times = append(b.times, now)
	return true
}

// evictOldest 淘汰最近最久未访问的 key（近似 LRU，用 seq 比较）。
func (rl *rateLimiter) evictOldest() {
	var oldestKey string
	var oldestSeq uint64
	first := true
	for k, b := range rl.entries {
		if first || b.seq < oldestSeq {
			oldestKey, oldestSeq = k, b.seq
			first = false
		}
	}
	if !first {
		delete(rl.entries, oldestKey)
	}
}
