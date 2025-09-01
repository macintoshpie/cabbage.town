package acls

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

func UpdateACLs(dryRun bool) error {
	if dryRun {
		log.Printf("[ACL] Starting ACL update process (DRY RUN)")
	} else {
		log.Printf("[ACL] Starting ACL update process")
	}

	log.Printf("[ACL] Loading credentials...")
	loadCredentials()

	// Initialize AWS session with DigitalOcean Spaces configuration
	log.Printf("[ACL] Initializing DigitalOcean Spaces session...")
	sess, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(
			getEnvOrPanic("DO_ACCESS_KEY_ID"),
			getEnvOrPanic("DO_SECRET_ACCESS_KEY"),
			"",
		),
		Endpoint:         aws.String("https://nyc3.digitaloceanspaces.com"),
		Region:           aws.String("us-east-1"),
		S3ForcePathStyle: aws.Bool(false),
	})
	if err != nil {
		log.Printf("[ACL] ERROR: Failed to create session: %v", err)
		return fmt.Errorf("failed to create session: %v", err)
	}
	log.Printf("[ACL] Successfully created DigitalOcean Spaces session")

	svc := s3.New(sess)
	users := []string{"ted", "brennan", "ben", "will"}
	bucketName := "cabbagetown"
	cutoffTime := time.Now().Add(-72 * time.Hour)

	log.Printf("[ACL] Checking for files modified after: %s", cutoffTime.Format(time.RFC3339))
	log.Printf("[ACL] Processing %d users: %v", len(users), users)

	var totalFilesChecked, totalFilesModified, totalFilesUpdated int
	var filesUpdated []FileChange

	for _, user := range users {
		prefix := fmt.Sprintf("recordings/%s/", user)
		log.Printf("[ACL] Checking recordings for user: %s (prefix: %s)", user, prefix)

		var userFilesChecked, userFilesModified, userFilesUpdated int

		log.Printf("[ACL] Listing objects for prefix: %s", prefix)
		err := svc.ListObjectsV2Pages(&s3.ListObjectsV2Input{
			Bucket: aws.String(bucketName),
			Prefix: aws.String(prefix),
		}, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
			log.Printf("[ACL] Processing page with %d objects", len(page.Contents))
			for _, obj := range page.Contents {
				userFilesChecked++
				totalFilesChecked++

				log.Printf("[ACL] Checking file: %s (Last modified: %s)", *obj.Key, obj.LastModified.Format(time.RFC3339))

				if obj.LastModified.After(cutoffTime) {
					userFilesModified++
					totalFilesModified++
					log.Printf("[ACL] File was modified within last 72 hours: %s", *obj.Key)

					log.Printf("[ACL] Getting ACL for file: %s", *obj.Key)
					aclOutput, err := svc.GetObjectAcl(&s3.GetObjectAclInput{
						Bucket: aws.String(bucketName),
						Key:    obj.Key,
					})
					if err != nil {
						log.Printf("[ACL] ERROR: Getting ACL for %s: %v", *obj.Key, err)
						continue
					}

					log.Printf("[ACL] Checking if file is private: %s", *obj.Key)
					isPrivate := true
					for _, grant := range aclOutput.Grants {
						if grant.Grantee.URI != nil && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
							isPrivate = false
							log.Printf("[ACL] File is already public: %s", *obj.Key)
							break
						}
					}

					if isPrivate {
						log.Printf("[ACL] File is private, checking for manual privacy metadata: %s", *obj.Key)
						headOutput, err := svc.HeadObject(&s3.HeadObjectInput{
							Bucket: aws.String(bucketName),
							Key:    obj.Key,
						})
						if err == nil && headOutput.Metadata != nil {
							if manuallyPrivated, ok := headOutput.Metadata["Manually-Privated"]; ok && *manuallyPrivated == "true" {
								log.Printf("[ACL] File has manually-privated=true metadata: %s", *obj.Key)
								// Simply respect the manual privacy setting
								continue
							} else {
								log.Printf("[ACL] File does not have manual privacy metadata: %s", *obj.Key)
							}
						} else {
							if err != nil {
								log.Printf("[ACL] WARNING: Could not get metadata for %s: %v", *obj.Key, err)
							} else {
								log.Printf("[ACL] File has no metadata: %s", *obj.Key)
							}
						}

						if dryRun {
							log.Printf("[ACL] DRY RUN: Would make public: %s", *obj.Key)
						} else {
							log.Printf("[ACL] Making file public: %s", *obj.Key)
							_, err = svc.PutObjectAcl(&s3.PutObjectAclInput{
								Bucket: aws.String(bucketName),
								Key:    obj.Key,
								ACL:    aws.String("public-read"),
							})
							if err != nil {
								log.Printf("[ACL] ERROR: Setting ACL for %s: %v", *obj.Key, err)
								continue
							}
							log.Printf("[ACL] Successfully made public: %s", *obj.Key)
						}
						userFilesUpdated++
						totalFilesUpdated++

						filesUpdated = append(filesUpdated, FileChange{
							Key:          *obj.Key,
							User:         user,
							LastModified: *obj.LastModified,
						})
					}
				} else {
					log.Printf("[ACL] File is older than 72 hours, skipping: %s", *obj.Key)
				}
			}
			return true
		})

		if err != nil {
			log.Printf("[ACL] ERROR: Listing objects for user %s: %v", user, err)
			continue
		}

		log.Printf("[ACL] Summary for user %s:", user)
		log.Printf("[ACL] - Files checked: %d", userFilesChecked)
		log.Printf("[ACL] - Files modified in last 72 hours: %d", userFilesModified)
		log.Printf("[ACL] - Files %s: %d", map[bool]string{true: "would be made public", false: "made public"}[dryRun], userFilesUpdated)
	}

	log.Printf("[ACL] Final Summary:")
	log.Printf("[ACL] - Total files checked: %d", totalFilesChecked)
	log.Printf("[ACL] - Total files modified in last 72 hours: %d", totalFilesModified)
	log.Printf("[ACL] - Total files %s: %d", map[bool]string{true: "would be made public", false: "made public"}[dryRun], totalFilesUpdated)

	if len(filesUpdated) > 0 {
		log.Printf("[ACL] Files %s in this run:", map[bool]string{true: "would be made public", false: "made public"}[dryRun])
		for _, file := range filesUpdated {
			log.Printf("[ACL] - [%s] %s (modified: %s)", file.User, file.Key, file.LastModified.Format(time.RFC3339))
		}
	} else {
		log.Printf("[ACL] No files were %s in this run", map[bool]string{true: "identified for public ACL", false: "made public"}[dryRun])
	}

	log.Printf("[ACL] ACL update process complete!")
	return nil
}

func getEnvOrPanic(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("[ACL] FATAL: Required environment variable %s is not set", key)
	}
	return value
}

func loadCredentials() {
	accessKey := os.Getenv("DO_ACCESS_KEY_ID")
	secretKey := os.Getenv("DO_SECRET_ACCESS_KEY")

	if accessKey == "" || secretKey == "" {
		log.Printf("[ACL] Credentials not found in environment, attempting to load from .env file...")

		projectRoot := filepath.Join(os.Getenv("HOME"), "Documents", "useless_stuff", "cabbage.town")
		log.Printf("[ACL] Looking for .env file in: %s", projectRoot)

		err := godotenv.Load(filepath.Join(projectRoot, ".env"))
		if err != nil {
			log.Printf("[ACL] WARNING: Could not load .env file: %v", err)
		} else {
			log.Printf("[ACL] Successfully loaded .env file")
		}
	} else {
		log.Printf("[ACL] Using credentials from environment variables")
	}

	log.Printf("[ACL] Verifying required credentials are available...")
	_ = getEnvOrPanic("DO_ACCESS_KEY_ID")
	_ = getEnvOrPanic("DO_SECRET_ACCESS_KEY")
	log.Printf("[ACL] Credentials verified successfully")
}
