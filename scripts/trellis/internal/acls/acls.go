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
		Region:           aws.String("us-east-1"),
		S3ForcePathStyle: aws.Bool(false),
	})
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}

	svc := s3.New(sess)
	users := []string{"ted", "brennan", "ben", "will"}
	bucketName := "cabbagetown"
	cutoffTime := time.Now().Add(-72 * time.Hour)

	log.Printf("Checking for files modified after: %s\n", cutoffTime.Format(time.RFC3339))

	var totalFilesChecked, totalFilesModified, totalFilesUpdated int
	var filesUpdated []FileChange

	for _, user := range users {
		prefix := fmt.Sprintf("recordings/%s/", user)
		log.Printf("Checking recordings for user: %s (prefix: %s)\n", user, prefix)

		var userFilesChecked, userFilesModified, userFilesUpdated int

		err := svc.ListObjectsV2Pages(&s3.ListObjectsV2Input{
			Bucket: aws.String(bucketName),
			Prefix: aws.String(prefix),
		}, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
			for _, obj := range page.Contents {
				userFilesChecked++
				totalFilesChecked++

				log.Printf("Checking file: %s (Last modified: %s)\n", *obj.Key, obj.LastModified.Format(time.RFC3339))

				if obj.LastModified.After(cutoffTime) {
					userFilesModified++
					totalFilesModified++
					log.Printf("File was modified within last 72 hours: %s\n", *obj.Key)

					aclOutput, err := svc.GetObjectAcl(&s3.GetObjectAclInput{
						Bucket: aws.String(bucketName),
						Key:    obj.Key,
					})
					if err != nil {
						log.Printf("Error getting ACL for %s: %v\n", *obj.Key, err)
						continue
					}

					isPrivate := true
					for _, grant := range aclOutput.Grants {
						if grant.Grantee.URI != nil && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
							isPrivate = false
							break
						}
					}

					if isPrivate {
						headOutput, err := svc.HeadObject(&s3.HeadObjectInput{
							Bucket: aws.String(bucketName),
							Key:    obj.Key,
						})
						if err == nil && headOutput.Metadata != nil {
							if manuallyPrivated, ok := headOutput.Metadata["manually-privated"]; ok && *manuallyPrivated == "true" {
								if privacyTimestamp, ok := headOutput.Metadata["privacy-timestamp"]; ok {
									if ts, err := time.Parse(time.RFC3339, *privacyTimestamp); err == nil {
										if ts.After(*obj.LastModified) {
											log.Printf("File was manually set to private, respecting setting: %s\n", *obj.Key)
											continue
										}
									}
								}
							}
						}

						if dryRun {
							log.Printf("Would make public: %s\n", *obj.Key)
						} else {
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
						}
						userFilesUpdated++
						totalFilesUpdated++

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
			return true
		})

		if err != nil {
			log.Printf("Error listing objects for user %s: %v\n", user, err)
			continue
		}

		log.Printf("Summary for user %s:\n", user)
		log.Printf("- Files checked: %d\n", userFilesChecked)
		log.Printf("- Files modified in last 72 hours: %d\n", userFilesModified)
		log.Printf("- Files %s: %d\n", map[bool]string{true: "would be made public", false: "made public"}[dryRun], userFilesUpdated)
	}

	log.Printf("Final Summary:\n")
	log.Printf("- Total files checked: %d\n", totalFilesChecked)
	log.Printf("- Total files modified in last 72 hours: %d\n", totalFilesModified)
	log.Printf("- Total files %s: %d\n", map[bool]string{true: "would be made public", false: "made public"}[dryRun], totalFilesUpdated)

	return nil
}

func getEnvOrPanic(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Required environment variable %s is not set", key)
	}
	return value
}

func loadCredentials() {
	accessKey := os.Getenv("DO_ACCESS_KEY_ID")
	secretKey := os.Getenv("DO_SECRET_ACCESS_KEY")

	if accessKey == "" || secretKey == "" {
		log.Println("Credentials not found in environment, attempting to load from .env file...")

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

	_ = getEnvOrPanic("DO_ACCESS_KEY_ID")
	_ = getEnvOrPanic("DO_SECRET_ACCESS_KEY")
}