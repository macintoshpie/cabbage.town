package posts

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"cabbage.town/shed.cabbage.town/pkg/bucket"
)

// Post represents a blog post (matching shed's structure)
type Post struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Slug      string       `json:"slug"`
	Markdown  string       `json:"markdown"`
	Author    string       `json:"author"`    // User-specified display name
	CreatedBy string       `json:"createdBy"` // Authenticated username (for permissions)
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
	Published bool         `json:"published"`
	DeletedAt *time.Time   `json:"deletedAt,omitempty"`
	Metadata  PostMetadata `json:"metadata"`
}

type PostMetadata struct {
	Tags      []string `json:"tags"`
	Category  string   `json:"category"`
	Excerpt   string   `json:"excerpt"`
	Recording string   `json:"recording"` // S3 key of associated recording
}

// Config holds configuration for post generation
type Config struct {
	BucketClient *bucket.Client
	OutputDir    string // JSON data files (posts.json, recordings.json)
	PlaylistsDir string // M3U playlist files
}

// UserPlaylist represents a user-specific playlist with filtering
type UserPlaylist struct {
	Username string
	Filename string
	Filter   func(Recording) bool
}

// Recording represents a recording from the trellis sync (matches trellis.Recording)
type Recording struct {
	URL          string
	Key          string
	DJ           string
	Show         string
	Date         string
	LastModified time.Time
	DisplayName  string
}

// PostOutput is the JSON-serializable output type for posts
type PostOutput struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Slug         string    `json:"slug"`
	Markdown     string    `json:"markdown"`
	Author       string    `json:"author"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Tags         []string  `json:"tags"`
	Category     string    `json:"category"`
	Excerpt      string    `json:"excerpt"`
	RecordingKey string    `json:"recordingKey,omitempty"`
}

// RecordingOutput is the JSON-serializable output type for recordings
type RecordingOutput struct {
	Key          string    `json:"key"`
	URL          string    `json:"url"`
	DJ           string    `json:"dj"`
	Show         string    `json:"show"`
	Date         string    `json:"date"`
	LastModified time.Time `json:"lastModified"`
	DisplayName  string    `json:"displayName"`
}

// ListPosts fetches all published, non-deleted posts from S3
func ListPosts(client *bucket.Client) ([]Post, error) {
	log.Printf("[POSTS] Listing posts from S3...")
	objects, err := client.ListObjects("posts/")
	if err != nil {
		return nil, fmt.Errorf("failed to list posts: %v", err)
	}

	log.Printf("[POSTS] Found %d objects with posts/ prefix", len(objects))

	var posts []Post
	for _, obj := range objects {
		if obj.Key == nil {
			continue
		}

		// Only process .json files
		if len(*obj.Key) < 5 || (*obj.Key)[len(*obj.Key)-5:] != ".json" {
			log.Printf("[POSTS] Skipping non-JSON file: %s", *obj.Key)
			continue
		}

		// Fetch and parse post
		output, err := client.GetObject(*obj.Key)
		if err != nil {
			log.Printf("[POSTS] WARNING: Failed to fetch %s: %v", *obj.Key, err)
			continue
		}

		data, err := ioutil.ReadAll(output.Body)
		output.Body.Close()
		if err != nil {
			log.Printf("[POSTS] WARNING: Failed to read %s: %v", *obj.Key, err)
			continue
		}

		var post Post
		if err := json.Unmarshal(data, &post); err != nil {
			log.Printf("[POSTS] WARNING: Failed to parse %s: %v", *obj.Key, err)
			continue
		}

		// Filter: only published, non-deleted posts
		if !post.Published {
			log.Printf("[POSTS] Skipping unpublished post: %s", post.Title)
			continue
		}
		if post.DeletedAt != nil {
			log.Printf("[POSTS] Skipping deleted post: %s", post.Title)
			continue
		}

		posts = append(posts, post)
		log.Printf("[POSTS] Added post: %s by %s", post.Title, post.Author)
	}

	// Sort by CreatedAt descending (newest first)
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].CreatedAt.After(posts[j].CreatedAt)
	})

	log.Printf("[POSTS] Returning %d published posts", len(posts))
	return posts, nil
}

// getShowName returns the show name and DJ name for a given username
func getShowName(dj string) (string, string, error) {
	switch dj {
	case "brennan":
		return "Late Nights Like These", "Nights Like These", nil
	case "ted":
		return "mulch channel", "dj ted", nil
	case "ben":
		return "IS WiLD hour", "DJ CHICAGO STYLE", nil
	case "will":
		return "tracks from terminus", "the conductor", nil
	case "katherine":
		return "The reginajingles show", "reginajingles", nil
	case "seth":
		return "Home Cooking Show", "Seth", nil
	default:
		return "", "", fmt.Errorf("unknown DJ: %s", dj)
	}
}

// parseRecordingInfo extracts recording information from a URL
func parseRecordingInfo(url string, lastModified time.Time) Recording {
	// Example URL: https://cabbagetown.nyc3.digitaloceanspaces.com/recordings/brennan/stream_20250626-204143.mp3
	parts := strings.Split(url, "/")

	var username, show, dj string
	var date time.Time

	// Extract username from URL path
	if len(parts) >= 5 {
		username = parts[4]
		show, dj, _ = getShowName(username)
	}

	filename := parts[len(parts)-1]

	// Try standard format first: stream_YYYYMMDD-HHMMSS.mp3
	if strings.HasPrefix(filename, "stream_") && len(filename) >= 23 {
		dateStr := filename[7:15] // Extract YYYYMMDD
		parsedDate, err := time.Parse("20060102", dateStr)
		if err == nil {
			date = parsedDate
		}
	}

	// Fallback: Try to find YYYYMMDD-HHMMSS pattern anywhere in filename
	if date.IsZero() {
		datePattern := regexp.MustCompile(`(\d{8})-\d{6}`)
		if matches := datePattern.FindStringSubmatch(filename); len(matches) > 1 {
			parsedDate, err := time.Parse("20060102", matches[1])
			if err == nil {
				date = parsedDate
			}
		}
	}

	// If still no date, use lastModified
	if date.IsZero() {
		date = lastModified
	}

	return Recording{
		URL:          url,
		DJ:           dj,
		Show:         show,
		Date:         date.Format("January 2, 2006"),
		LastModified: date,
	}
}

// isRecordingPublic checks if a recording has public-read ACL
func isRecordingPublic(client *bucket.Client, key string) bool {
	aclOutput, err := client.GetObjectACL(key)
	if err != nil {
		log.Printf("[POSTS] WARNING: Failed to get ACL for %s: %v", key, err)
		return false
	}

	// Check if the object has public-read ACL
	for _, grant := range aclOutput.Grants {
		if grant.Grantee.URI != nil && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
			return true
		}
	}

	return false
}

// fetchRecordingsFromS3 fetches all public recordings directly from the S3 bucket
func fetchRecordingsFromS3(client *bucket.Client) ([]Recording, error) {
	log.Printf("[POSTS] Fetching recordings from S3...")
	objects, err := client.ListObjects("recordings/")
	if err != nil {
		return nil, fmt.Errorf("failed to list recordings: %v", err)
	}
	log.Printf("[POSTS] Found %d objects in recordings/", len(objects))

	var recordings []Recording
	var skipped int
	var privateCount int

	for _, obj := range objects {
		if obj.Key == nil || len(*obj.Key) < 4 || (*obj.Key)[len(*obj.Key)-4:] != ".mp3" {
			continue
		}

		// Check if recording is public
		if !isRecordingPublic(client, *obj.Key) {
			log.Printf("[POSTS] Skipping private recording: %s", *obj.Key)
			privateCount++
			continue
		}

		// Construct URL
		fullURL := "https://cabbagetown.nyc3.digitaloceanspaces.com/" + *obj.Key
		log.Printf("[POSTS] Processing public MP3: %s", *obj.Key)

		// Parse recording info (handles both standard and custom formats)
		lastModified := time.Now()
		if obj.LastModified != nil {
			lastModified = *obj.LastModified
		}

		recording := parseRecordingInfo(fullURL, lastModified)
		recording.Key = *obj.Key

		// Get object metadata to check for display name
		headOutput, err := client.HeadObject(*obj.Key)
		if err == nil && headOutput.Metadata != nil {
			if displayName, ok := headOutput.Metadata["Display-Name"]; ok && displayName != nil {
				recording.DisplayName = *displayName
				log.Printf("[POSTS] Using display name: %s", recording.DisplayName)
			}
		}

		// If no display name from metadata, use the show name
		if recording.DisplayName == "" {
			recording.DisplayName = recording.Show
		}

		recordings = append(recordings, recording)
	}

	// Sort by LastModified descending (newest first), then by Key for stability
	sort.Slice(recordings, func(i, j int) bool {
		if recordings[i].LastModified.Equal(recordings[j].LastModified) {
			return recordings[i].Key < recordings[j].Key
		}
		return recordings[i].LastModified.After(recordings[j].LastModified)
	})

	log.Printf("[POSTS] Successfully fetched %d public recordings (skipped %d, private %d)", len(recordings), skipped, privateCount)
	return recordings, nil
}

// generatePlaylist creates an M3U playlist file from recordings
func generatePlaylist(recordings []Recording, outputFile string, outputDir string, filter func(Recording) bool) error {
	outputFilePath := filepath.Join(outputDir, outputFile)

	if err := os.MkdirAll(filepath.Dir(outputFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	content := "#EXTM3U\n"
	for _, recording := range recordings {
		if filter == nil || filter(recording) {
			title := fmt.Sprintf("%s - %s", recording.DisplayName, recording.Date)
			content += fmt.Sprintf("#EXTINF:-1,%s\n%s\n", title, recording.URL)
		}
	}

	if err := os.WriteFile(outputFilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write playlist file: %v", err)
	}

	return nil
}

// GeneratePlaylists creates M3U playlist files from recordings
func GeneratePlaylists(recordings []Recording, outputDir string) error {
	log.Printf("[POSTS] Generating playlists...")

	userPlaylists := []UserPlaylist{
		{
			Username: "seth",
			Filename: filepath.Join("playlists", "home_cooking.m3u"),
			Filter: func(r Recording) bool {
				return r.DJ == "Seth"
			},
		},
		{
			Username: "will",
			Filename: filepath.Join("playlists", "tracks_from_terminus.m3u"),
			Filter: func(r Recording) bool {
				return r.DJ == "the conductor"
			},
		},
	}

	// Generate main playlist with all recordings
	mainPlaylist := filepath.Join("playlists", "recordings.m3u")
	if err := generatePlaylist(recordings, mainPlaylist, outputDir, nil); err != nil {
		return fmt.Errorf("failed to generate main playlist: %v", err)
	}
	log.Printf("[POSTS] Generated main playlist with %d recordings", len(recordings))

	// Generate user-specific playlists
	for _, userPlaylist := range userPlaylists {
		matchingCount := 0
		for _, r := range recordings {
			if userPlaylist.Filter(r) {
				matchingCount++
			}
		}

		if err := generatePlaylist(recordings, userPlaylist.Filename, outputDir, userPlaylist.Filter); err != nil {
			return fmt.Errorf("failed to generate playlist for user %s: %v", userPlaylist.Username, err)
		}
		log.Printf("[POSTS] Generated playlist for %s with %d recordings", userPlaylist.Username, matchingCount)
	}

	log.Printf("[POSTS] Successfully generated all playlists")
	return nil
}

// Run fetches posts and recordings from S3 and writes JSON data files
func Run(config Config) error {
	log.Printf("[POSTS] Starting data export process")

	// List all published posts
	posts, err := ListPosts(config.BucketClient)
	if err != nil {
		return fmt.Errorf("failed to list posts: %v", err)
	}

	// Convert posts to output format
	postOutputs := make([]PostOutput, len(posts))
	for i, p := range posts {
		postOutputs[i] = PostOutput{
			ID:           p.ID,
			Title:        p.Title,
			Slug:         p.Slug,
			Markdown:     p.Markdown,
			Author:       p.Author,
			CreatedAt:    p.CreatedAt,
			UpdatedAt:    p.UpdatedAt,
			Tags:         p.Metadata.Tags,
			Category:     p.Metadata.Category,
			Excerpt:      p.Metadata.Excerpt,
			RecordingKey: p.Metadata.Recording,
		}
	}

	// Ensure output directory exists
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Write posts.json
	postsJSON, err := json.MarshalIndent(postOutputs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal posts: %v", err)
	}
	postsFile := fmt.Sprintf("%s/posts.json", config.OutputDir)
	if err := os.WriteFile(postsFile, postsJSON, 0644); err != nil {
		return fmt.Errorf("failed to write posts.json: %v", err)
	}
	log.Printf("[POSTS] Wrote %d posts to %s", len(postOutputs), postsFile)

	// Fetch recordings from S3
	recordings, err := fetchRecordingsFromS3(config.BucketClient)
	if err != nil {
		log.Printf("[POSTS] WARNING: Failed to fetch recordings from S3: %v", err)
		log.Printf("[POSTS] Skipping recordings.json")
		log.Printf("[POSTS] Data export complete: %d posts", len(posts))
		return nil
	}

	// Convert recordings to output format
	recOutputs := make([]RecordingOutput, len(recordings))
	for i, r := range recordings {
		recOutputs[i] = RecordingOutput{
			Key:          r.Key,
			URL:          r.URL,
			DJ:           r.DJ,
			Show:         r.Show,
			Date:         r.Date,
			LastModified: r.LastModified,
			DisplayName:  r.DisplayName,
		}
	}

	// Write recordings.json
	recJSON, err := json.MarshalIndent(recOutputs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal recordings: %v", err)
	}
	recFile := fmt.Sprintf("%s/recordings.json", config.OutputDir)
	if err := os.WriteFile(recFile, recJSON, 0644); err != nil {
		return fmt.Errorf("failed to write recordings.json: %v", err)
	}
	log.Printf("[POSTS] Wrote %d recordings to %s", len(recOutputs), recFile)

	// Generate playlists
	if err := GeneratePlaylists(recordings, config.PlaylistsDir); err != nil {
		return fmt.Errorf("failed to generate playlists: %v", err)
	}

	log.Printf("[POSTS] Data export complete: %d posts, %d recordings", len(posts), len(recordings))
	return nil
}
