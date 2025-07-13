package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

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

	if *dryRun {
		fmt.Println("🔍 DRY RUN: Update recordings workflow")
	} else {
		fmt.Println("🎵 Starting recordings update workflow...")
	}

	// Step 1: Update ACLs for recent recordings
	if !*skipACL {
		fmt.Println("\n📋 Step 1: Updating ACLs for recent recordings...")
		err := acls.UpdateACLs(*dryRun)
		if err != nil {
			fmt.Printf("Error updating ACLs: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("\n📋 Step 1: Skipping ACL updates")
	}

	// Step 2: Add ID3 metadata to recent recordings
	if !*skipMetadata {
		fmt.Println("\n🏷️  Step 2: Adding ID3 metadata to recent recordings...")
		err := metadata.UpdateMetadata(*dryRun)
		if err != nil {
			fmt.Printf("Error updating metadata: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("\n🏷️  Step 2: Skipping metadata updates")
	}

	// Step 3: Generate playlists and RSS feed
	if !*skipPlaylists {
		fmt.Println("\n📻 Step 3: Updating playlists and RSS feed...")
		if *dryRun {
			fmt.Println("   Would regenerate playlists and RSS feed")
		} else {
			err := updatePlaylists()
			if err != nil {
				fmt.Printf("Error updating playlists: %v\n", err)
				os.Exit(1)
			}
		}
	} else {
		fmt.Println("\n📻 Step 3: Skipping playlist updates")
	}

	if *dryRun {
		fmt.Println("\n✅ Dry run complete - no changes were made")
	} else {
		fmt.Println("\n✅ Recordings update workflow complete!")
	}
}

func updatePlaylists() error {
	// Use existing trellis logic
	outputDir := filepath.Join("..", "..", "public")
	outputFile := filepath.Join("playlists", "recordings.m3u")
	sethPlaylistFile := filepath.Join("playlists", "home_cooking.m3u")
	rssFile := filepath.Join("feed.xml")

	config := trellis.Config{
		BucketURL:        "https://cabbagetown.nyc3.digitaloceanspaces.com",
		ListURL:          "https://cabbagetown.nyc3.digitaloceanspaces.com/?prefix=recordings/&max-keys=1000",
		OutputDir:        outputDir,
		OutputFile:       outputFile,
		SethPlaylistFile: sethPlaylistFile,
		RSSFile:          rssFile,
	}

	return trellis.Run(config)
}