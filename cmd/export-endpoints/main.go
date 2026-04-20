package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/babbage88/infra-core/deployment"
)

func main() {
	outputPath := flag.String("out", "", "Path to write endpoints JSON")
	flag.Parse()

	if *outputPath == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}

	payload, err := json.MarshalIndent(deployment.EndpointSpecs(), "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal endpoint specs: %v\n", err)
		os.Exit(1)
	}
	payload = append(payload, '\n')

	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create output directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outputPath, payload, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write endpoint specs: %v\n", err)
		os.Exit(1)
	}
}
