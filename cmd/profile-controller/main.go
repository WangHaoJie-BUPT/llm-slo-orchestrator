package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bupt20220407/llm-slo-orchestrator/internal/profile"
)

var version = "w3-dev"

func main() {
	showVersion := flag.Bool("version", false, "print version")
	policyPath := flag.String("policy", "config/example-policy.yaml", "strict InferenceSLOPolicy path")
	profilePath := flag.String("profile", "config/qwen3-0.6b-profile.yaml", "strict ModelBackendProfile path")
	outputPath := flag.String("output", "config/reconciled-profile.json", "reconciliation status output")
	interval := flag.Duration("interval", 2*time.Second, "continuous reconciliation interval")
	once := flag.Bool("once", false, "reconcile once and exit")
	check := flag.Bool("check", false, "verify that the output matches the inputs")
	flag.Parse()
	if *showVersion {
		fmt.Printf("llm-slo-profile-controller %s\n", version)
		return
	}
	if *interval <= 0 {
		log.Fatal("reconciliation interval must be positive")
	}
	if *once && *check {
		log.Fatal("--once and --check are mutually exclusive")
	}

	reconcile := func(write bool) error {
		status, err := profile.Reconcile(*policyPath, *profilePath)
		if err != nil {
			return err
		}
		if !write {
			if err := profile.CheckStatus(*outputPath, status); err != nil {
				return err
			}
			log.Printf("reconciliation status is current: model=%s policy_generation=%d profile_generation=%d", status.Model, status.PolicyGeneration, status.ProfileGeneration)
			return nil
		}
		changed, err := profile.WriteStatus(*outputPath, status)
		if err != nil {
			return err
		}
		if changed {
			log.Printf("reconciled model=%s policy_generation=%d profile_generation=%d", status.Model, status.PolicyGeneration, status.ProfileGeneration)
		}
		return nil
	}

	if *check {
		if err := reconcile(false); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := reconcile(true); err != nil {
		log.Fatal(err)
	}
	if *once {
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := reconcile(true); err != nil {
				log.Printf("reconciliation failed: %v", err)
			}
		}
	}
}
