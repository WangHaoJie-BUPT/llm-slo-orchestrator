package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"inference.bupt.dev/llm-slo-orchestrator/internal/backend"
	"inference.bupt.dev/llm-slo-orchestrator/internal/config"
	"inference.bupt.dev/llm-slo-orchestrator/internal/picker"
	server "inference.bupt.dev/llm-slo-orchestrator/internal/router"
)

var version = "w2-dev"

type backendFlags []string

func (flags *backendFlags) String() string {
	return strings.Join(*flags, ",")
}

func (flags *backendFlags) Set(value string) error {
	*flags = append(*flags, value)
	return nil
}

func main() {
	showVersion := flag.Bool("version", false, "print version")
	checkConfig := flag.String("check-config", "", "check that a configuration file is readable")
	configPath := flag.String("config", "config/example-policy.yaml", "routing policy path")
	listenAddress := flag.String("listen", "127.0.0.1:8080", "HTTP listen address")
	healthInterval := flag.Duration("health-interval", 2*time.Second, "backend health probe interval")
	healthTimeout := flag.Duration("health-timeout", time.Second, "backend health probe timeout")
	healthFailureThreshold := flag.Int("health-failure-threshold", 2, "consecutive failures before removal")
	healthSuccessThreshold := flag.Int("health-success-threshold", 1, "consecutive successes before recovery")
	var backends backendFlags
	flag.Var(&backends, "backend", "backend target in id=http://host:port form; repeat for multiple targets")
	flag.Parse()

	if *showVersion {
		fmt.Printf("llm-slo-router %s\n", version)
		return
	}

	if *checkConfig != "" {
		document, _, err := config.Load(*checkConfig)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("configuration is valid: %s (%s)\n", *checkConfig, document.Metadata.Name)
		return
	}

	_, policy, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load policy: %v", err)
	}
	policy.Generation = 1
	selectedPicker, err := picker.New(policy.Strategy)
	if err != nil {
		log.Fatalf("create picker: %v", err)
	}
	targets, err := parseBackends(backends, policy.Model)
	if err != nil {
		log.Fatalf("configure backends: %v", err)
	}
	logger := log.New(os.Stdout, "router ", log.LstdFlags|log.LUTC)
	handler, err := server.New(policy, selectedPicker, targets, logger)
	if err != nil {
		log.Fatalf("create router: %v", err)
	}
	healthMonitor := backend.HealthMonitor{
		Interval:         *healthInterval,
		Timeout:          *healthTimeout,
		FailureThreshold: *healthFailureThreshold,
		SuccessThreshold: *healthSuccessThreshold,
	}
	if err := healthMonitor.Validate(); err != nil {
		log.Fatalf("configure health monitor: %v", err)
	}

	httpServer := &http.Server{
		Addr:              *listenAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	healthTargets := make([]backend.HealthTarget, 0, len(targets))
	for _, target := range targets {
		healthTargets = append(healthTargets, backend.HealthTarget{
			ID:               target.ID,
			BaseURL:          target.BaseURL,
			InitiallyHealthy: target.Healthy,
		})
	}
	go func() {
		err := healthMonitor.Run(ctx, healthTargets, func(update backend.HealthUpdate) {
			if updateErr := handler.SetEndpointHealth(update.ID, update.Healthy, update.CheckedAt); updateErr != nil {
				logger.Printf("health update error: %v", updateErr)
			}
		})
		if err != nil && ctx.Err() == nil {
			logger.Printf("health monitor stopped: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if shutdownErr := httpServer.Shutdown(shutdownContext); shutdownErr != nil {
			logger.Printf("shutdown error: %v", shutdownErr)
		}
	}()

	logger.Printf("listening=%s strategy=%s backends=%d", *listenAddress, policy.Strategy, len(targets))
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
}

func parseBackends(values []string, model string) ([]server.TargetConfig, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("at least one --backend is required")
	}
	targets := make([]server.TargetConfig, 0, len(values))
	for _, value := range values {
		id, backendURL, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(backendURL) == "" {
			return nil, fmt.Errorf("invalid backend %q; expected id=http://host:port", value)
		}
		targets = append(targets, server.TargetConfig{
			ID:      strings.TrimSpace(id),
			Model:   model,
			BaseURL: strings.TrimSpace(backendURL),
			Healthy: true,
			Ready:   true,
		})
	}
	return targets, nil
}
