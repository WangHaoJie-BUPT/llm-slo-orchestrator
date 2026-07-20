package main

import (
	"flag"
	"fmt"
	"os"

	"inference.bupt.dev/llm-slo-orchestrator/internal/config"
)

var version = "w0-dev"

func main() {
	showVersion := flag.Bool("version", false, "print version")
	checkConfig := flag.String("check-config", "", "check that a configuration file is readable")
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

	fmt.Println("router command contract initialized; serving lands in W1/W2")
}
