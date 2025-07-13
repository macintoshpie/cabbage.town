package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/joho/godotenv"

	"cabbage.town/shed.cabbage.town/pkg/bucket"
	"cabbage.town/trellis/trellis"
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

	// Load .env file from parent directory
	if err := godotenv.Load("../../.env"); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// Initialize bucket client
	bucketClient, err := bucket.NewClient()
	if err != nil {
		fmt.Printf("Error creating bucket client: %v\n", err)
		os.Exit(1)
	}

	config := trellis.Config{
		BucketURL: "https://cabbagetown.nyc3.digitaloceanspaces.com",
		ListURL:   "https://cabbagetown.nyc3.digitaloceanspaces.com/?prefix=recordings/&max-keys=1000",
	}

	allRecordings, err := trellis.ListRecordings(config)
	if err != nil {
		fmt.Printf("Error listing recordings: %v\n", err)
		os.Exit(1)
	}

	// Filter to only recent recordings for metadata processing
	recentRecordings := trellis.FilterRecentRecordings(allRecordings)
	fmt.Printf("Found %d recent recordings (last 72 hours) out of %d total\n", len(recentRecordings), len(allRecordings))

	for _, recording := range recentRecordings {
		err := addID3Metadata(recording, bucketClient, *dryRun)
		if err != nil {
			fmt.Printf("Failed to add metadata to %s: %v\n", recording.URL, err)
			continue
		}
		if *dryRun {
			fmt.Printf("Would add metadata to %s\n", recording.URL)
		} else {
			fmt.Printf("Added metadata to %s\n", recording.URL)
		}
	}

	fmt.Println("ID3 metadata processing complete.")
}

func addID3Metadata(recording trellis.Recording, bucketClient *bucket.Client, dryRun bool) error {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "id3_processing")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %v", err)
	}
	
	// Only clean up temp dir if not in dry run mode
	if !dryRun {
		defer os.RemoveAll(tempDir)
	}

	// Use stored key from listing
	key := recording.Key
	filename := filepath.Base(key)
	tempFile := filepath.Join(tempDir, filename)

	// Get existing object metadata and ACL
	headOutput, err := bucketClient.HeadObject(key)
	if err != nil {
		return fmt.Errorf("failed to get object metadata: %v", err)
	}

	// Check if already processed
	if processed, exists := headOutput.Metadata["id3-processed"]; exists && *processed == "true" {
		fmt.Printf("  Skipping %s - already processed\n", key)
		return nil
	}

	aclOutput, err := bucketClient.GetObjectACL(key)
	if err != nil {
		return fmt.Errorf("failed to get object ACL: %v", err)
	}

	// Download file using bucket client
	obj, err := bucketClient.GetObject(key)
	if err != nil {
		return fmt.Errorf("failed to get object: %v", err)
	}
	defer obj.Body.Close()

	// Write to temp file
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer file.Close()

	_, err = file.ReadFrom(obj.Body)
	if err != nil {
		return fmt.Errorf("failed to write temp file: %v", err)
	}
	file.Close()

	// Parse date for year
	date, err := time.Parse("January 02, 2006", recording.Date)
	if err != nil {
		return fmt.Errorf("failed to parse date: %v", err)
	}
	year := fmt.Sprintf("%d", date.Year())

	// Add ID3 metadata using eyeD3
	cmd := exec.Command("eyeD3",
		"-t", fmt.Sprintf("%s (%s)", recording.Show, recording.Date),
		"-a", recording.DJ,
		"-A", "Cabbage Town Radio",
		"-Y", year,
		"-G", "Electronic",
		"-c", "Live recording from Cabbage Town Radio",
		tempFile,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("eyeD3 failed: %v, output: %s", err, string(output))
	}

	if dryRun {
		// For dry run, just log where the file would be saved
		fmt.Printf("  Dry run: Modified file saved locally at: %s\n", tempFile)
		fmt.Printf("  Would upload to key: %s\n", key)
		fmt.Printf("  Would add id3-processed=true to metadata\n")
	} else {
		// Prepare updated metadata - copy existing and add processed flag
		updatedMetadata := make(map[string]*string)
		for k, v := range headOutput.Metadata {
			updatedMetadata[k] = v
		}
		updatedMetadata["id3-processed"] = aws.String("true")

		// Determine ACL from existing permissions
		acl := "private" // default
		for _, grant := range aclOutput.Grants {
			if grant.Grantee.URI != nil && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
				acl = "public-read"
				break
			}
		}

		// Upload modified file back with preserved metadata and ACL
		modifiedFile, err := os.Open(tempFile)
		if err != nil {
			return fmt.Errorf("failed to open modified file: %v", err)
		}
		defer modifiedFile.Close()

		err = bucketClient.PutObjectWithMetadata(key, modifiedFile, "audio/mpeg", updatedMetadata, acl)
		if err != nil {
			return fmt.Errorf("failed to upload file: %v", err)
		}
	}

	return nil
}
