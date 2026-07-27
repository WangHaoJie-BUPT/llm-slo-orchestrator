package admission

import (
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (selected *fakeClock) Now() time.Time { return selected.now }

func TestLimiterBurstRefillAndTenantIsolation(t *testing.T) {
	selectedClock := &fakeClock{now: time.Unix(100, 0)}
	limiter, err := New(2, 2, selectedClock)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if allowed, _ := limiter.Allow("a"); !allowed {
			t.Fatal("burst request was rejected")
		}
	}
	if allowed, retry := limiter.Allow("a"); allowed || retry != time.Second {
		t.Fatalf("exhausted bucket = %t, %s", allowed, retry)
	}
	if allowed, _ := limiter.Allow("b"); !allowed {
		t.Fatal("one tenant exhausted another tenant's bucket")
	}
	selectedClock.now = selectedClock.now.Add(500 * time.Millisecond)
	if allowed, _ := limiter.Allow("a"); !allowed {
		t.Fatal("refilled token was rejected")
	}
}
