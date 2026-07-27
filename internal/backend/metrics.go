package backend

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MetricsUpdate struct {
	ID          string
	Running     int
	Waiting     int
	KVUsage     float64
	TTFTEWMA    time.Duration
	CollectedAt time.Time
}

type MetricsMonitor struct {
	Interval time.Duration
	Timeout  time.Duration
	Client   *http.Client
}

type metricSample struct {
	running, waiting int
	kv, ttftSum      float64
	ttftCount        float64
	hasRunning       bool
	hasWaiting       bool
	hasKV            bool
}

func (monitor MetricsMonitor) Run(ctx context.Context, targets []HealthTarget, update func(MetricsUpdate)) error {
	if monitor.Interval <= 0 || monitor.Timeout <= 0 || len(targets) == 0 || update == nil {
		return fmt.Errorf("metrics interval, timeout, targets, and callback are required")
	}
	client := monitor.Client
	if client == nil {
		client = &http.Client{Timeout: monitor.Timeout}
	}
	metricsURLs := make([]string, len(targets))
	for index, target := range targets {
		metricsURL, err := targetURL(target, "/metrics")
		if err != nil {
			return err
		}
		metricsURLs[index] = metricsURL
	}
	var group sync.WaitGroup
	for index, target := range targets {
		metricsURL := metricsURLs[index]
		group.Add(1)
		go func() {
			defer group.Done()
			monitor.runTarget(ctx, client, target.ID, metricsURL, update)
		}()
	}
	group.Wait()
	return nil
}

func (monitor MetricsMonitor) runTarget(ctx context.Context, client *http.Client, id, metricsURL string, update func(MetricsUpdate)) {
	var previous metricSample
	var hasPrevious bool
	var ewma time.Duration
	collect := func() {
		sample, err := scrapeMetrics(ctx, client, metricsURL)
		if err != nil {
			return
		}
		if hasPrevious && sample.ttftCount > previous.ttftCount {
			seconds := (sample.ttftSum - previous.ttftSum) / (sample.ttftCount - previous.ttftCount)
			observed := time.Duration(seconds * float64(time.Second))
			if observed > 0 {
				if ewma == 0 {
					ewma = observed
				} else {
					ewma = time.Duration(0.2*float64(observed) + 0.8*float64(ewma))
				}
			}
		}
		previous, hasPrevious = sample, true
		update(MetricsUpdate{ID: id, Running: sample.running, Waiting: sample.waiting, KVUsage: sample.kv, TTFTEWMA: ewma, CollectedAt: time.Now()})
	}
	collect()
	ticker := time.NewTicker(monitor.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collect()
		}
	}
}

func scrapeMetrics(ctx context.Context, client *http.Client, metricsURL string) (metricSample, error) {
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, nil)
	response, err := client.Do(request)
	if err != nil {
		return metricSample{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return metricSample{}, fmt.Errorf("metrics status %d", response.StatusCode)
	}
	return parseMetrics(io.LimitReader(response.Body, 8<<20))
}

func parseMetrics(reader io.Reader) (metricSample, error) {
	var result metricSample
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		nameEnd := strings.IndexAny(line, "{ ")
		if nameEnd < 0 {
			continue
		}
		name := line[:nameEnd]
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			continue
		}
		switch name {
		case "vllm:num_requests_running":
			result.running, result.hasRunning = int(value), true
		case "vllm:num_requests_waiting":
			result.waiting, result.hasWaiting = int(value), true
		case "vllm:kv_cache_usage_perc":
			result.kv, result.hasKV = value, true
		case "vllm:time_to_first_token_seconds_sum":
			result.ttftSum = value
		case "vllm:time_to_first_token_seconds_count":
			result.ttftCount = value
		}
	}
	if err := scanner.Err(); err != nil {
		return metricSample{}, err
	}
	if !result.hasRunning || !result.hasWaiting || !result.hasKV || result.kv < 0 || result.kv > 1 {
		return metricSample{}, fmt.Errorf("required vLLM metrics are missing or invalid")
	}
	return result, nil
}

func targetURL(target HealthTarget, path string) (string, error) {
	parsed, err := url.Parse(target.BaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("target %q has invalid URL", target.ID)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	parsed.RawQuery, parsed.Fragment = "", ""
	return parsed.String(), nil
}
