package backend

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type HealthTarget struct {
	ID               string
	BaseURL          string
	InitiallyHealthy bool
}

type HealthUpdate struct {
	ID        string
	Healthy   bool
	CheckedAt time.Time
}

type HealthMonitor struct {
	Interval         time.Duration
	Timeout          time.Duration
	FailureThreshold int
	SuccessThreshold int
	Client           *http.Client
}

func (monitor HealthMonitor) Validate() error {
	if monitor.Interval <= 0 {
		return fmt.Errorf("health interval must be positive")
	}
	if monitor.Timeout <= 0 {
		return fmt.Errorf("health timeout must be positive")
	}
	if monitor.FailureThreshold <= 0 || monitor.SuccessThreshold <= 0 {
		return fmt.Errorf("health thresholds must be positive")
	}
	return nil
}

func (monitor HealthMonitor) Run(ctx context.Context, targets []HealthTarget, update func(HealthUpdate)) error {
	if err := monitor.Validate(); err != nil {
		return err
	}
	if update == nil {
		return fmt.Errorf("health update callback is required")
	}
	if len(targets) == 0 {
		return fmt.Errorf("at least one health target is required")
	}
	client := monitor.Client
	if client == nil {
		client = &http.Client{Timeout: monitor.Timeout}
	}

	healthURLs := make([]string, len(targets))
	for index, target := range targets {
		healthURL, err := targetHealthURL(target)
		if err != nil {
			return err
		}
		healthURLs[index] = healthURL
	}

	var waitGroup sync.WaitGroup
	for index, target := range targets {
		healthURL := healthURLs[index]
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			monitor.runTarget(ctx, client, target, healthURL, update)
		}()
	}
	waitGroup.Wait()
	return nil
}

func (monitor HealthMonitor) runTarget(
	ctx context.Context,
	client *http.Client,
	target HealthTarget,
	healthURL string,
	update func(HealthUpdate),
) {
	state := healthState{healthy: target.InitiallyHealthy}
	check := func() {
		checkedAt := time.Now()
		succeeded := probe(ctx, client, healthURL)
		healthy := state.observe(succeeded, monitor.FailureThreshold, monitor.SuccessThreshold)
		update(HealthUpdate{ID: target.ID, Healthy: healthy, CheckedAt: checkedAt})
	}
	check()
	ticker := time.NewTicker(monitor.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

type healthState struct {
	healthy   bool
	successes int
	failures  int
}

func (state *healthState) observe(succeeded bool, failureThreshold, successThreshold int) bool {
	if succeeded {
		state.failures = 0
		state.successes++
		if state.successes >= successThreshold {
			state.healthy = true
		}
	} else {
		state.successes = 0
		state.failures++
		if state.failures >= failureThreshold {
			state.healthy = false
		}
	}
	return state.healthy
}

func targetHealthURL(target HealthTarget) (string, error) {
	if strings.TrimSpace(target.ID) == "" {
		return "", fmt.Errorf("health target ID is required")
	}
	parsed, err := url.Parse(target.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("health target %q has invalid URL %q", target.ID, target.BaseURL)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/health"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func probe(ctx context.Context, client *http.Client, healthURL string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return false
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
}
