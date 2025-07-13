package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"cabbage.town/shed.cabbage.town/pkg/bucket"
	"cabbage.town/trellis/internal/acls"
	"cabbage.town/trellis/internal/metadata"
	"cabbage.town/trellis/trellis"
)

func main() {
	// Parse command line flags
	dryRun := flag.Bool("dry-run", false, "Perform a dry run without making changes")
	skipACL := flag.Bool("skip-acl", false, "Skip ACL updates")
	skipMetadata := flag.Bool("skip-metadata", false, "Skip ID3 metadata processing")
	skipPlaylists := flag.Bool("skip-playlists", false, "Skip playlist/feed generation")
	flag.Parse()

	// Get subcommand from args
	args := flag.Args()
	var subcommand string
	if len(args) > 0 {
		subcommand = args[0]
	}

	// Validate subcommand
	switch subcommand {
	case "", "all":
		// Run all steps (default behavior)
	case "acls":
		*skipMetadata = true
		*skipPlaylists = true
	case "metadata":
		*skipACL = true
		*skipPlaylists = true
	case "playlists":
		*skipACL = true
		*skipMetadata = true
	default:
		fmt.Printf("Error: Unknown subcommand '%s'\n", subcommand)
		fmt.Println("Usage: update_recordings [OPTIONS] [SUBCOMMAND]")
		fmt.Println("")
		fmt.Println("Subcommands:")
		fmt.Println("  all        Run all steps (default)")
		fmt.Println("  acls       Update ACLs for recent recordings only")
		fmt.Println("  metadata   Add ID3 metadata to recent recordings only")
		fmt.Println("  playlists  Generate playlists and RSS feed only")
		fmt.Println("")
		fmt.Println("Options:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if subcommand == "" {
		subcommand = "all"
	}

	if *dryRun {
		log.Printf("[WORKFLOW] 🔍 Starting recordings update workflow: %s (DRY RUN)", subcommand)
	} else {
		log.Printf("[WORKFLOW] 🎵 Starting recordings update workflow: %s", subcommand)
	}

	log.Printf("[WORKFLOW] Configuration:")
	log.Printf("[WORKFLOW] - Subcommand: %s", subcommand)
	log.Printf("[WORKFLOW] - Dry run: %v", *dryRun)
	log.Printf("[WORKFLOW] - Skip ACL: %v", *skipACL)
	log.Printf("[WORKFLOW] - Skip metadata: %v", *skipMetadata)
	log.Printf("[WORKFLOW] - Skip playlists: %v", *skipPlaylists)

	// Initialize shared bucket client
	log.Printf("[WORKFLOW] Initializing shared bucket client...")
	bucketClient, err := bucket.NewClient()
	if err != nil {
		log.Printf("[WORKFLOW] ERROR: Failed to create bucket client: %v", err)
		os.Exit(1)
	}
	log.Printf("[WORKFLOW] Successfully created shared bucket client")

	// Step 1: Update ACLs for recent recordings
	if !*skipACL {
		log.Printf("[WORKFLOW] 📋 Step 1: Updating ACLs for recent recordings...")
		err := acls.UpdateACLs(*dryRun)
		if err != nil {
			log.Printf("[WORKFLOW] ERROR: Step 1 failed: %v", err)
			os.Exit(1)
		}
		log.Printf("[WORKFLOW] ✅ Step 1 completed successfully")
	} else {
		log.Printf("[WORKFLOW] ⏭️  Step 1: Skipping ACL updates")
	}

	// Step 2: Add ID3 metadata to recent recordings
	if !*skipMetadata {
		log.Printf("[WORKFLOW] 🏷️  Step 2: Adding ID3 metadata to recent recordings...")
		err := metadata.UpdateMetadata(bucketClient, *dryRun)
		if err != nil {
			log.Printf("[WORKFLOW] ERROR: Step 2 failed: %v", err)
			os.Exit(1)
		}
		log.Printf("[WORKFLOW] ✅ Step 2 completed successfully")
	} else {
		log.Printf("[WORKFLOW] ⏭️  Step 2: Skipping metadata updates")
	}

	// Step 3: Generate playlists and RSS feed
	if !*skipPlaylists {
		log.Printf("[WORKFLOW] 📻 Step 3: Updating playlists and RSS feed...")
		if *dryRun {
			log.Printf("[WORKFLOW] DRY RUN: Would regenerate playlists and RSS feed")
		} else {
			err := updatePlaylists(bucketClient)
			if err != nil {
				log.Printf("[WORKFLOW] ERROR: Step 3 failed: %v", err)
				os.Exit(1)
			}
		}
		log.Printf("[WORKFLOW] ✅ Step 3 completed successfully")
	} else {
		log.Printf("[WORKFLOW] ⏭️  Step 3: Skipping playlist updates")
	}

	if *dryRun {
		log.Printf("[WORKFLOW] 🎯 Dry run complete - no changes were made")
	} else {
		log.Printf("[WORKFLOW] 🎉 Recordings update workflow complete!")
	}
}

func updatePlaylists(bucketClient *bucket.Client) error {
	// Use existing trellis logic
	outputDir := filepath.Join("..", "..", "public")
	outputFile := filepath.Join("playlists", "recordings.m3u")
	sethPlaylistFile := filepath.Join("playlists", "home_cooking.m3u")
	rssFile := filepath.Join("feed.xml")

	config := trellis.Config{
		BucketClient:     bucketClient,
		OutputDir:        outputDir,
		OutputFile:       outputFile,
		SethPlaylistFile: sethPlaylistFile,
		RSSFile:          rssFile,
	}

	return trellis.Run(config)
}
