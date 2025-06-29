package main

import (
	"fmt"
	"path/filepath"

	"cabbage.town/trellis/trellis"
)

func main() {
	// Define the configuration with paths relative to the current working directory
	outputDir := filepath.Join("..", "..", "public")
	outputFile := filepath.Join("playlists", "recordings.m3u")
	rssFile := filepath.Join("feed.xml")

	config := trellis.Config{
		BucketURL:  "https://cabbagetown.nyc3.digitaloceanspaces.com",
		ListURL:    "https://cabbagetown.nyc3.digitaloceanspaces.com/?prefix=recordings/&max-keys=1000",
		OutputDir:  outputDir,
		OutputFile: outputFile,
		RSSFile:    rssFile,
	}

	// Run the trellis command with the configuration
	err := trellis.Run(config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
