package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bupt20220407/llm-slo-orchestrator/internal/config"
	"github.com/bupt20220407/llm-slo-orchestrator/internal/console"
)

var version = "w5-dev"

func main() {
	showVersion := flag.Bool("version", false, "print version")
	listenAddress := flag.String("listen", "127.0.0.1:8090", "HTTP listen address")
	routerURL := flag.String("router-url", "http://127.0.0.1:8080", "router base URL")
	policyPath := flag.String("policy", "config/example-policy.yaml", "routing policy path")
	repositoryRoot := flag.String("repository-root", ".", "repository root containing reports and evidence")
	dashboardURL := flag.String("dashboard-url", "http://127.0.0.1:3000/d/llm-slo-overview", "Grafana dashboard URL")
	demoMode := flag.Bool("demo-mode", false, "enable token-protected local strategy and scenario controls")
	flag.Parse()
	if *showVersion {
		fmt.Printf("llm-slo-console %s\n", version)
		return
	}
	_, policy, err := config.Load(*policyPath)
	if err != nil {
		log.Fatalf("load policy: %v", err)
	}
	server, err := console.New(console.Config{
		RouterURL:          *routerURL,
		DashboardURL:       *dashboardURL,
		RepositoryRoot:     *repositoryRoot,
		Model:              policy.Model,
		DemoMode:           *demoMode,
		ControlToken:       os.Getenv("LLM_SLO_CONSOLE_CONTROL_TOKEN"),
		RouterControlToken: os.Getenv("LLM_SLO_ROUTER_CONTROL_TOKEN"),
	})
	if err != nil {
		log.Fatalf("create console: %v", err)
	}
	httpServer := &http.Server{
		Addr:              *listenAddress,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if shutdownErr := httpServer.Shutdown(shutdownContext); shutdownErr != nil {
			log.Printf("shutdown error: %v", shutdownErr)
		}
	}()
	log.Printf("console listening=%s router=%s demo_mode=%t", *listenAddress, *routerURL, *demoMode)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
}
