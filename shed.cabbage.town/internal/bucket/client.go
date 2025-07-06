package bucket

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

const (
	BucketName = "cabbagetown"
	endpoint   = "https://nyc3.digitaloceanspaces.com"
	region     = "us-east-1"
)

// User represents a user in the system
type User struct {
	Password string `json:"password"`
	IsAdmin  bool   `json:"isAdmin"`
}

// UserStore represents the collection of users
type UserStore struct {
	Users map[string]User `json:"users"`
}

// Client wraps an S3 client with our bucket-specific operations
type Client struct {
	s3Client *s3.S3
	bucket   string
}

// NewClient creates a new bucket client
func NewClient() (*Client, error) {
	accessKey := os.Getenv("DO_ACCESS_KEY_ID")
	secretKey := os.Getenv("DO_SECRET_ACCESS_KEY")

	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("DO_ACCESS_KEY_ID and DO_SECRET_ACCESS_KEY must be set")
	}

	sess, err := session.NewSession(&aws.Config{
		Credentials:      credentials.NewStaticCredentials(accessKey, secretKey, ""),
		Endpoint:         aws.String(endpoint),
		Region:           aws.String(region),
		S3ForcePathStyle: aws.Bool(false),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}

	return &Client{
		s3Client: s3.New(sess),
		bucket:   BucketName,
	}, nil
}

// GetObject retrieves an object from the bucket
func (c *Client) GetObject(key string) (*s3.GetObjectOutput, error) {
	return c.s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
}

// PutObject uploads an object to the bucket
func (c *Client) PutObject(key string, body []byte, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String(contentType),
	}

	_, err := c.s3Client.PutObject(input)
	return err
}

// GetObjectACL gets the ACL for an object
func (c *Client) GetObjectACL(key string) (*s3.GetObjectAclOutput, error) {
	return c.s3Client.GetObjectAcl(&s3.GetObjectAclInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
}

// PutObjectACL sets the ACL for an object
func (c *Client) PutObjectACL(key string, acl string) error {
	_, err := c.s3Client.PutObjectAcl(&s3.PutObjectAclInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		ACL:    aws.String(acl),
	})
	return err
}

// ListObjects lists objects with the given prefix
func (c *Client) ListObjects(prefix string) ([]*s3.Object, error) {
	var objects []*s3.Object

	err := c.s3Client.ListObjectsV2Pages(&s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(prefix),
	}, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
		objects = append(objects, page.Contents...)
		return true
	})

	return objects, err
}

// GetPresignedURL generates a presigned URL for an object that expires after the specified duration
func (c *Client) GetPresignedURL(key string, expires time.Duration) (string, error) {
	req, _ := c.s3Client.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})

	return req.Presign(expires)
}

// PutObjectStreaming uploads a file from a reader to the bucket
func (c *Client) PutObjectStreaming(key string, reader io.Reader, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        aws.ReadSeekCloser(reader),
		ContentType: aws.String(contentType),
	}

	_, err := c.s3Client.PutObject(input)
	return err
}

// GetPresignedPutURL generates a presigned URL for uploading a file
func (c *Client) GetPresignedPutURL(key, contentType string, expires time.Duration) (string, error) {
	req, _ := c.s3Client.PutObjectRequest(&s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	})
	return req.Presign(expires)
}
