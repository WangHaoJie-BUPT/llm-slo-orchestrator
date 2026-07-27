package admission

import (
	"fmt"
	"math"
	"sync"
	"time"

	"inference.bupt.dev/llm-slo-orchestrator/internal/clock"
)

const maxTenants = 4096

type Limiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	clock   clock.Clock
	buckets map[string]bucket
}

type bucket struct {
	tokens   float64
	updated  time.Time
	lastSeen time.Time
}

func New(rate float64, burst int, selectedClock clock.Clock) (*Limiter, error) {
	if rate <= 0 || burst <= 0 {
		return nil, fmt.Errorf("tenant rate and burst must be positive")
	}
	if selectedClock == nil {
		return nil, fmt.Errorf("clock is required")
	}
	return &Limiter{rate: rate, burst: float64(burst), clock: selectedClock, buckets: make(map[string]bucket)}, nil
}

func (limiter *Limiter) Allow(tenant string) (bool, time.Duration) {
	now := limiter.clock.Now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	current, exists := limiter.buckets[tenant]
	if !exists {
		if len(limiter.buckets) >= maxTenants {
			limiter.removeIdle(now)
		}
		if len(limiter.buckets) >= maxTenants {
			return false, time.Second
		}
		current = bucket{tokens: limiter.burst, updated: now}
	}
	elapsed := now.Sub(current.updated).Seconds()
	if elapsed > 0 {
		current.tokens = math.Min(limiter.burst, current.tokens+elapsed*limiter.rate)
	}
	current.updated = now
	current.lastSeen = now
	if current.tokens >= 1 {
		current.tokens--
		limiter.buckets[tenant] = current
		return true, 0
	}
	limiter.buckets[tenant] = current
	retry := time.Duration(math.Ceil((1-current.tokens)/limiter.rate)) * time.Second
	if retry < time.Second {
		retry = time.Second
	}
	return false, retry
}

func (limiter *Limiter) removeIdle(now time.Time) {
	idleTTL := time.Duration(math.Ceil(2*limiter.burst/limiter.rate)) * time.Second
	if idleTTL < time.Minute {
		idleTTL = time.Minute
	}
	for tenant, current := range limiter.buckets {
		if now.Sub(current.lastSeen) > idleTTL {
			delete(limiter.buckets, tenant)
		}
	}
}
