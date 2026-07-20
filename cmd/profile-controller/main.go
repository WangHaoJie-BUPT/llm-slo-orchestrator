package main

import (
	"flag"
	"fmt"
)

var version = "w0-dev"

func main() {
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("llm-slo-profile-controller %s\n", version)
		return
	}

	fmt.Println("profile-controller command contract initialized; reconciliation lands after W1")
}
