package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/joho/godotenv"
)

// FileChange tracks changes made to a file
type FileChange struct {
	Key          string
	User         string
	LastModified time.Time
}

// getEnvOrPanic gets an environment variable or panics if it's not set
func getEnvOrPanic(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Required environment variable %s is not set", key)
	}
	return value
}

// loadCredentials attempts to load credentials from environment variables first,
// then falls back to .env file if needed
func loadCredentials() {
	// Check if credentials are already set in environment
	accessKey := os.Getenv("DO_ACCESS_KEY_ID")
	secretKey := os.Getenv("DO_SECRET_ACCESS_KEY")

	// If either credential is missing, try loading from .env
	if accessKey == "" || secretKey == "" {
		log.Println("Credentials not found in environment, attempting to load from .env file...")

		// Load .env file from project root
		projectRoot := filepath.Join(os.Getenv("HOME"), "Documents", "useless_stuff", "cabbage.town")
		log.Printf("Looking for .env file in: %s\n", projectRoot)

		err := godotenv.Load(filepath.Join(projectRoot, ".env"))
		if err != nil {
			log.Printf("Warning: Could not load .env file: %v\n", err)
		} else {
			log.Println("Successfully loaded .env file")
		}
	} else {
		log.Println("Using credentials from environment variables")
	}

	// Verify we have the required credentials (either from env or .env)
	_ = getEnvOrPanic("DO_ACCESS_KEY_ID")
	_ = getEnvOrPanic("DO_SECRET_ACCESS_KEY")
}

func main() {
	log.Println("Starting ACL update process...")

	// Load credentials from environment or .env
	loadCredentials()

	// Initialize AWS session with DigitalOcean Spaces configuration
	log.Println("Initializing DigitalOcean Spaces session...")
	sess, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(
			getEnvOrPanic("DO_ACCESS_KEY_ID"),
			getEnvOrPanic("DO_SECRET_ACCESS_KEY"),
			"",
		),
		Endpoint:         aws.String("https://nyc3.digitaloceanspaces.com"),
		Region:           aws.String("us-east-1"), // Required for AWS SDK
		S3ForcePathStyle: aws.Bool(false),         // Required for DigitalOcean Spaces
	})
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}
	log.Println("Successfully created DigitalOcean Spaces session")

	// Create S3 service client
	svc := s3.New(sess)

	// Users to check
	users := []string{"ted", "brennan", "ben", "will"}
	bucketName := "cabbagetown"

	// Get current time for 72-hour comparison
	cutoffTime := time.Now().Add(-72 * time.Hour)
	log.Printf("Checking for files modified after: %s\n", cutoffTime.Format(time.RFC3339))

	var totalFilesChecked, totalFilesModified, totalFilesUpdated int
	var filesUpdated []FileChange

	// Process each user's recordings
	for _, user := range users {
		prefix := fmt.Sprintf("recordings/%s/", user)
		log.Printf("\nChecking recordings for user: %s (prefix: %s)\n", user, prefix)

		var userFilesChecked, userFilesModified, userFilesUpdated int

		// List objects in the user's recordings folder
		err := svc.ListObjectsV2Pages(&s3.ListObjectsV2Input{
			Bucket: aws.String(bucketName),
			Prefix: aws.String(prefix),
		}, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
			for _, obj := range page.Contents {
				userFilesChecked++
				totalFilesChecked++

				log.Printf("Checking file: %s (Last modified: %s)\n", *obj.Key, obj.LastModified.Format(time.RFC3339))

				// Check if object was modified in last 72 hours
				if obj.LastModified.After(cutoffTime) {
					userFilesModified++
					totalFilesModified++
					log.Printf("File was modified within last 72 hours: %s\n", *obj.Key)

					// Get current ACL
					aclOutput, err := svc.GetObjectAcl(&s3.GetObjectAclInput{
						Bucket: aws.String(bucketName),
						Key:    obj.Key,
					})
					if err != nil {
						log.Printf("Error getting ACL for %s: %v\n", *obj.Key, err)
						continue
					}

					// Check if object is private by looking for public-read grant
					isPrivate := true
					for _, grant := range aclOutput.Grants {
						if grant.Grantee.URI != nil && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
							isPrivate = false
							break
						}
					}

					if isPrivate {
						log.Printf("File is private, making public: %s\n", *obj.Key)
						_, err = svc.PutObjectAcl(&s3.PutObjectAclInput{
							Bucket: aws.String(bucketName),
							Key:    obj.Key,
							ACL:    aws.String("public-read"),
						})
						if err != nil {
							log.Printf("Error setting ACL for %s: %v\n", *obj.Key, err)
							continue
						}
						log.Printf("Successfully made public: %s\n", *obj.Key)
						userFilesUpdated++
						totalFilesUpdated++

						// Only track files we actually made public
						filesUpdated = append(filesUpdated, FileChange{
							Key:          *obj.Key,
							User:         user,
							LastModified: *obj.LastModified,
						})
					} else {
						log.Printf("File is already public: %s\n", *obj.Key)
					}
				} else {
					log.Printf("File is older than 72 hours, skipping: %s\n", *obj.Key)
				}
			}
			return true // continue paging
		})

		if err != nil {
			log.Printf("Error listing objects for user %s: %v\n", user, err)
			continue
		}

		log.Printf("\nSummary for user %s:\n", user)
		log.Printf("- Files checked: %d\n", userFilesChecked)
		log.Printf("- Files modified in last 72 hours: %d\n", userFilesModified)
		log.Printf("- Files made public: %d\n", userFilesUpdated)
	}

	log.Printf("\nFinal Summary:\n")
	log.Printf("- Total files checked: %d\n", totalFilesChecked)
	log.Printf("- Total files modified in last 72 hours: %d\n", totalFilesModified)
	log.Printf("- Total files made public: %d\n", totalFilesUpdated)

	if len(filesUpdated) > 0 {
		log.Printf("\nFiles made public in this run:\n")
		for _, file := range filesUpdated {
			log.Printf("- [%s] %s (modified: %s)\n",
				file.User,
				file.Key,
				file.LastModified.Format(time.RFC3339),
			)
		}
	} else {
		log.Println("\nNo files were made public in this run.")
	}

	log.Println("ACL update process complete!")
}
