package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"

	"cabbage.town/shed.cabbage.town/pkg/bucket"
	"cabbage.town/trellis/internal/posts"
)

func main() {
	// Parse command line flags
	dryRun := flag.Bool("dry-run", false, "Perform a dry run without making changes")
	flag.Parse()

	if *dryRun {
		log.Printf("[UPDATE_POSTS] 🔍 Starting post update (DRY RUN)")
	} else {
		log.Printf("[UPDATE_POSTS] 📝 Starting post update")
	}

	// Load environment variables from .env file
	log.Printf("[UPDATE_POSTS] Loading environment variables...")
	if err := godotenv.Load("../../.env"); err != nil {
		log.Printf("[UPDATE_POSTS] WARNING: Could not load .env file: %v", err)
		log.Printf("[UPDATE_POSTS] Will attempt to use environment variables directly")
	} else {
		log.Printf("[UPDATE_POSTS] Successfully loaded .env file")
	}

	// Initialize bucket client
	log.Printf("[UPDATE_POSTS] Initializing bucket client...")
	bucketClient, err := bucket.NewClient()
	if err != nil {
		log.Printf("[UPDATE_POSTS] ERROR: Failed to create bucket client: %v", err)
		log.Printf("[UPDATE_POSTS] Please ensure DO_ACCESS_KEY_ID and DO_SECRET_ACCESS_KEY are set")
		os.Exit(1)
	}
	log.Printf("[UPDATE_POSTS] Successfully created bucket client")

	if *dryRun {
		log.Printf("[UPDATE_POSTS] DRY RUN: Would generate static post pages and posts.json")
		log.Printf("[UPDATE_POSTS] 🎯 Dry run complete - no changes were made")
		return
	}

	// Generate static posts
	outputDir := filepath.Join("..", "..", "public")
	config := posts.Config{
		BucketClient: bucketClient,
		OutputDir:    outputDir,
	}

	log.Printf("[UPDATE_POSTS] Generating static post pages...")
	if err := posts.Run(config); err != nil {
		log.Printf("[UPDATE_POSTS] ERROR: Failed to generate posts: %v", err)
		os.Exit(1)
	}

	log.Printf("[UPDATE_POSTS] 🎉 Post update complete!")
}
