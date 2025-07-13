package main

import (
	"flag"
	"fmt"
	"os"

	"cabbage.town/trellis/internal/metadata"
)

func main() {
	// Parse command line flags
	dryRun := flag.Bool("dry-run", false, "Perform a dry run without uploading files")
	flag.Parse()

	if *dryRun {
		fmt.Println("🏷️  dry run: adding ID3 metadata to recordings (no uploads)...")
	} else {
		fmt.Println("🏷️  adding ID3 metadata to recordings...")
	}

	err := metadata.UpdateMetadata(*dryRun)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
