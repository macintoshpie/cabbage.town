package metadata

import (
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

func UpdateMetadata(dryRun bool) error {
	if dryRun {
		log.Printf("[METADATA] Starting ID3 metadata update process (DRY RUN)")
	} else {
		log.Printf("[METADATA] Starting ID3 metadata update process")
	}

	// Load .env file from parent directory
	log.Printf("[METADATA] Loading .env file from parent directory...")
	if err := godotenv.Load("../../.env"); err != nil {
		log.Printf("[METADATA] WARNING: Could not load .env file: %v", err)
	} else {
		log.Printf("[METADATA] Successfully loaded .env file")
	}

	// Initialize bucket client
	log.Printf("[METADATA] Initializing bucket client...")
	bucketClient, err := bucket.NewClient()
	if err != nil {
		log.Printf("[METADATA] ERROR: Creating bucket client: %v", err)
		return fmt.Errorf("error creating bucket client: %v", err)
	}
	log.Printf("[METADATA] Successfully created bucket client")

	config := trellis.Config{
		BucketURL: "https://cabbagetown.nyc3.digitaloceanspaces.com",
		ListURL:   "https://cabbagetown.nyc3.digitaloceanspaces.com/?prefix=recordings/&max-keys=1000",
	}

	log.Printf("[METADATA] Listing all recordings...")
	allRecordings, err := trellis.ListRecordings(config)
	if err != nil {
		log.Printf("[METADATA] ERROR: Listing recordings: %v", err)
		return fmt.Errorf("error listing recordings: %v", err)
	}
	log.Printf("[METADATA] Found %d total recordings", len(allRecordings))

	// Filter to only recent recordings for metadata processing
	log.Printf("[METADATA] Filtering to recent recordings (last 72 hours)...")
	recentRecordings := trellis.FilterRecentRecordings(allRecordings)
	log.Printf("[METADATA] Found %d recent recordings (last 72 hours) out of %d total", len(recentRecordings), len(allRecordings))

	var processed, skipped, failed int
	for i, recording := range recentRecordings {
		log.Printf("[METADATA] Processing recording %d/%d: %s", i+1, len(recentRecordings), recording.Key)
		err := addID3Metadata(recording, bucketClient, dryRun)
		if err != nil {
			log.Printf("[METADATA] ERROR: Failed to add metadata to %s: %v", recording.URL, err)
			failed++
			continue
		}
		if dryRun {
			log.Printf("[METADATA] DRY RUN: Would add metadata to %s", recording.URL)
		} else {
			log.Printf("[METADATA] Successfully added metadata to %s", recording.URL)
		}
		processed++
	}

	log.Printf("[METADATA] Summary:")
	log.Printf("[METADATA] - Total recent recordings: %d", len(recentRecordings))
	log.Printf("[METADATA] - Successfully processed: %d", processed)
	log.Printf("[METADATA] - Failed: %d", failed)
	log.Printf("[METADATA] - Skipped (already processed): %d", skipped)
	log.Printf("[METADATA] ID3 metadata processing complete")
	return nil
}

func addID3Metadata(recording trellis.Recording, bucketClient *bucket.Client, dryRun bool) error {
	log.Printf("[METADATA] Processing file: %s", recording.Key)
	
	// Create temporary directory
	log.Printf("[METADATA] Creating temporary directory...")
	tempDir, err := os.MkdirTemp("", "id3_processing")
	if err != nil {
		log.Printf("[METADATA] ERROR: Creating temp dir: %v", err)
		return fmt.Errorf("failed to create temp dir: %v", err)
	}
	log.Printf("[METADATA] Created temp directory: %s", tempDir)
	
	// Only clean up temp dir if not in dry run mode
	if !dryRun {
		defer func() {
			log.Printf("[METADATA] Cleaning up temp directory: %s", tempDir)
			os.RemoveAll(tempDir)
		}()
	} else {
		log.Printf("[METADATA] DRY RUN: Temp directory will be preserved: %s", tempDir)
	}

	// Use stored key from listing
	key := recording.Key
	filename := filepath.Base(key)
	tempFile := filepath.Join(tempDir, filename)
	log.Printf("[METADATA] Target temp file: %s", tempFile)

	// Get existing object metadata and ACL
	log.Printf("[METADATA] Getting object metadata for: %s", key)
	headOutput, err := bucketClient.HeadObject(key)
	if err != nil {
		log.Printf("[METADATA] ERROR: Getting object metadata: %v", err)
		return fmt.Errorf("failed to get object metadata: %v", err)
	}
	log.Printf("[METADATA] Retrieved metadata, found %d metadata fields", len(headOutput.Metadata))

	// Check if already processed
	if processed, exists := headOutput.Metadata["id3-processed"]; exists && *processed == "true" {
		log.Printf("[METADATA] File already processed, skipping: %s", key)
		return nil
	}
	log.Printf("[METADATA] File not yet processed, proceeding with ID3 metadata addition")

	log.Printf("[METADATA] Getting object ACL for: %s", key)
	aclOutput, err := bucketClient.GetObjectACL(key)
	if err != nil {
		log.Printf("[METADATA] ERROR: Getting object ACL: %v", err)
		return fmt.Errorf("failed to get object ACL: %v", err)
	}
	log.Printf("[METADATA] Retrieved ACL, found %d grants", len(aclOutput.Grants))

	// Download file using bucket client
	log.Printf("[METADATA] Downloading file from bucket: %s", key)
	obj, err := bucketClient.GetObject(key)
	if err != nil {
		log.Printf("[METADATA] ERROR: Getting object from bucket: %v", err)
		return fmt.Errorf("failed to get object: %v", err)
	}
	defer obj.Body.Close()
	log.Printf("[METADATA] Successfully retrieved object from bucket")

	// Write to temp file
	log.Printf("[METADATA] Creating local temp file: %s", tempFile)
	file, err := os.Create(tempFile)
	if err != nil {
		log.Printf("[METADATA] ERROR: Creating temp file: %v", err)
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer file.Close()

	log.Printf("[METADATA] Writing object data to temp file...")
	bytesWritten, err := file.ReadFrom(obj.Body)
	if err != nil {
		log.Printf("[METADATA] ERROR: Writing temp file: %v", err)
		return fmt.Errorf("failed to write temp file: %v", err)
	}
	file.Close()
	log.Printf("[METADATA] Successfully wrote %d bytes to temp file", bytesWritten)

	// Parse date for year
	log.Printf("[METADATA] Parsing recording date: %s", recording.Date)
	date, err := time.Parse("January 02, 2006", recording.Date)
	if err != nil {
		log.Printf("[METADATA] ERROR: Parsing date: %v", err)
		return fmt.Errorf("failed to parse date: %v", err)
	}
	year := fmt.Sprintf("%d", date.Year())
	log.Printf("[METADATA] Extracted year: %s", year)

	// Add ID3 metadata using eyeD3
	title := fmt.Sprintf("%s (%s)", recording.Show, recording.Date)
	log.Printf("[METADATA] Preparing to add ID3 metadata:")
	log.Printf("[METADATA] - Title: %s", title)
	log.Printf("[METADATA] - Artist: %s", recording.DJ)
	log.Printf("[METADATA] - Album: Cabbage Town Radio")
	log.Printf("[METADATA] - Year: %s", year)
	log.Printf("[METADATA] - Genre: Electronic")
	
	cmd := exec.Command("eyeD3",
		"-t", title,
		"-a", recording.DJ,
		"-A", "Cabbage Town Radio",
		"-Y", year,
		"-G", "Electronic",
		"-c", "Live recording from Cabbage Town Radio",
		tempFile,
	)

	log.Printf("[METADATA] Executing eyeD3 command: %v", cmd.Args)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[METADATA] ERROR: eyeD3 failed: %v, output: %s", err, string(output))
		return fmt.Errorf("eyeD3 failed: %v, output: %s", err, string(output))
	}
	log.Printf("[METADATA] eyeD3 completed successfully, output: %s", string(output))

	if dryRun {
		// For dry run, just log where the file would be saved
		log.Printf("[METADATA] DRY RUN: Modified file saved locally at: %s", tempFile)
		log.Printf("[METADATA] DRY RUN: Would upload to key: %s", key)
		log.Printf("[METADATA] DRY RUN: Would add id3-processed=true to metadata")
		log.Printf("[METADATA] DRY RUN: Processing complete for %s", key)
	} else {
		// Prepare updated metadata - copy existing and add processed flag
		log.Printf("[METADATA] Preparing updated metadata...")
		updatedMetadata := make(map[string]*string)
		for k, v := range headOutput.Metadata {
			updatedMetadata[k] = v
		}
		updatedMetadata["id3-processed"] = aws.String("true")
		log.Printf("[METADATA] Added id3-processed=true flag, total metadata fields: %d", len(updatedMetadata))

		// Determine ACL from existing permissions
		log.Printf("[METADATA] Determining ACL from existing permissions...")
		acl := "private" // default
		for _, grant := range aclOutput.Grants {
			if grant.Grantee.URI != nil && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
				acl = "public-read"
				log.Printf("[METADATA] File has public-read ACL")
				break
			}
		}
		if acl == "private" {
			log.Printf("[METADATA] File has private ACL")
		}

		// Upload modified file back with preserved metadata and ACL
		log.Printf("[METADATA] Opening modified file for upload: %s", tempFile)
		modifiedFile, err := os.Open(tempFile)
		if err != nil {
			log.Printf("[METADATA] ERROR: Opening modified file: %v", err)
			return fmt.Errorf("failed to open modified file: %v", err)
		}
		defer modifiedFile.Close()

		log.Printf("[METADATA] Uploading modified file with metadata and ACL: %s", key)
		err = bucketClient.PutObjectWithMetadata(key, modifiedFile, "audio/mpeg", updatedMetadata, acl)
		if err != nil {
			log.Printf("[METADATA] ERROR: Uploading file: %v", err)
			return fmt.Errorf("failed to upload file: %v", err)
		}
		log.Printf("[METADATA] Successfully uploaded modified file: %s", key)
	}

	log.Printf("[METADATA] Processing complete for file: %s", key)
	return nil
}