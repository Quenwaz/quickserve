package upload

import (
	"testing"
	"time"
)

func TestRateLimiterBasic(t *testing.T) {
	rl := newRateLimiter(3, time.Minute)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !rl.Allow("1.1.1.1", now) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if rl.Allow("1.1.1.1", now) {
		t.Error("4th request should be denied")
	}
	// 其他 IP 不受影响。
	if !rl.Allow("2.2.2.2", now) {
		t.Error("different IP should be allowed")
	}
}

func TestRateLimiterWindowSlide(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)
	now := time.Now()
	if !rl.Allow("ip", now) || !rl.Allow("ip", now) {
		t.Fatal("first two requests should be allowed")
	}
	if rl.Allow("ip", now) {
		t.Fatal("third request should be denied")
	}
	// 窗口滑过后恢复。
	later := now.Add(time.Minute + time.Second)
	if !rl.Allow("ip", later) {
		t.Error("request after window should be allowed")
	}
}

func TestRateLimiterUnlimited(t *testing.T) {
	rl := newRateLimiter(0, time.Minute)
	now := time.Now()
	for i := 0; i < 10000; i++ {
		if !rl.Allow("ip", now) {
			t.Fatalf("request %d should be allowed when unlimited", i+1)
		}
	}
	if len(rl.entries) != 0 {
		t.Errorf("entries = %d, want 0 (unlimited should not track)", len(rl.entries))
	}
}

func TestRateLimiterEviction(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	rl.maxKeys = 3
	now := time.Now()
	for i := 0; i < 10; i++ {
		rl.Allow(string(rune('a'+i)), now)
	}
	if len(rl.entries) > 3 {
		t.Errorf("entries = %d, want <= 3 after eviction", len(rl.entries))
	}
}
