package posts

import (
	"bytes"
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

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"

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

// PostListItem represents a post in the public listing
type PostListItem struct {
	Title   string `json:"title"`
	Slug    string `json:"slug"`
	Author  string `json:"author"`
	Date    string `json:"date"` // Formatted date string
	Excerpt string `json:"excerpt"`
}

// Config holds configuration for post generation
type Config struct {
	BucketClient *bucket.Client
	OutputDir    string
}

// UserPlaylist represents a user-specific playlist with filtering
type UserPlaylist struct {
	Username string
	Filename string
	Filter   func(Recording) bool
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

// generateRecordingPlayer creates an HTML audio player if the post has an associated recording
func generateRecordingPlayer(post Post) string {
	if post.Metadata.Recording == "" {
		return ""
	}

	// Generate the URL for the recording
	recordingURL := fmt.Sprintf("https://cabbagetown.nyc3.digitaloceanspaces.com/%s", post.Metadata.Recording)

	return fmt.Sprintf(`
            <div class="post-container recording-player">
                <h3>Listen to this show</h3>
                <audio controls>
                    <source src="%s" type="audio/mpeg">
                    Your browser does not support the audio element.
                </audio>
            </div>
`, recordingURL)
}

// GeneratePostHTML creates an HTML file for a single post
func GeneratePostHTML(post Post, outputDir string) error {
	// Convert markdown to HTML using goldmark
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM), // GitHub Flavored Markdown
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(post.Markdown), &buf); err != nil {
		return fmt.Errorf("failed to convert markdown: %v", err)
	}

	// Format date
	formattedDate := post.CreatedAt.Format("January 2, 2006")

	// Generate HTML page with cabbage.town styling
	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>%s - cabbage.town</title>
    <link rel="icon" href="../icon.ico" type="image/x-icon">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link rel="stylesheet" href="../reset.css">
    <link rel="stylesheet" href="../common.css">
    <link rel="stylesheet" href="../post.css">
    <style>
        .content {
            max-width: 700px;
            width: 100%%;
        }
    </style>
</head>
<body>

    <div class="main">
        <a href="/">
            <img src="../the-cabbage.png" style="width: 80px; height: auto" alt="cabbage" id="logo">
        </a>
        <a href="/" style="text-decoration: none; color: black;">
            <h2 class="syne-mono-regular">cabbage.town</h2>
        </a>

        <div class="content">
            <a href="/" class="back-link">back to home</a>
            %s
            <div class="post-container">
                <div class="post-header">
                    <h1 class="post-title">%s</h1>
                    <div class="post-meta">
                        <strong>%s</strong> - %s
                    </div>
                </div>
                
                <div class="post-content">
                    %s
                </div>
            </div>

            <a href="/" class="back-link">back to home</a>
        </div>
    </div>
</body>
</html>
`, post.Title, generateRecordingPlayer(post), post.Title, post.Author, formattedDate, buf.String())

	// Write to file
	patchDir := filepath.Join(outputDir, "patch")
	if err := os.MkdirAll(patchDir, 0755); err != nil {
		return fmt.Errorf("failed to create patch directory: %v", err)
	}

	outputFile := filepath.Join(patchDir, fmt.Sprintf("%s.html", post.Slug))
	if err := ioutil.WriteFile(outputFile, []byte(htmlContent), 0644); err != nil {
		return fmt.Errorf("failed to write HTML file: %v", err)
	}

	log.Printf("[POSTS] Generated HTML: %s", outputFile)
	return nil
}

// RecordingNoteInfo represents the post information for a recording
type RecordingNoteInfo struct {
	PostSlug  string `json:"postSlug"`
	PostTitle string `json:"postTitle"`
	Excerpt   string `json:"excerpt"`
}

// ShowItem represents a unified item that can be a recording, a post, or both
type ShowItem struct {
	Title     string         `json:"title"`
	Author    string         `json:"author"`
	Date      string         `json:"date"` // Formatted date string
	Timestamp time.Time      `json:"-"`    // For sorting, not exported
	Recording *RecordingInfo `json:"recording,omitempty"`
	Post      *PostInfo      `json:"post,omitempty"`
}

type RecordingInfo struct {
	Key string `json:"key"`
	URL string `json:"url"`
}

type PostInfo struct {
	Slug    string `json:"slug"`
	Excerpt string `json:"excerpt"`
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

	// Sort by LastModified descending (newest first)
	sort.Slice(recordings, func(i, j int) bool {
		return recordings[i].LastModified.After(recordings[j].LastModified)
	})

	log.Printf("[POSTS] Successfully fetched %d public recordings (skipped %d, private %d)", len(recordings), skipped, privateCount)
	return recordings, nil
}

// generatePlaylist creates an M3U playlist file from recordings
func generatePlaylist(recordings []Recording, outputFile string, outputDir string, filter func(Recording) bool) error {
	// Create directory for output file
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	outputFilePath := filepath.Join(outputDir, outputFile)

	// Initialize playlist with M3U header
	content := "#EXTM3U\n"

	// Add filtered recordings to playlist
	for _, recording := range recordings {
		if filter == nil || filter(recording) {
			// For playlists, include the date to distinguish episodes
			title := fmt.Sprintf("%s - %s", recording.DisplayName, recording.Date)
			content += fmt.Sprintf("#EXTINF:-1,%s\n%s\n", title, recording.URL)
		}
	}

	if err := ioutil.WriteFile(outputFilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write playlist file: %v", err)
	}

	return nil
}

// GeneratePlaylists creates M3U playlist files from recordings
func GeneratePlaylists(recordings []Recording, outputDir string) error {
	log.Printf("[POSTS] Generating playlists...")

	// Define user playlists
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
	log.Printf("[POSTS] Generating main playlist: %s", mainPlaylist)
	if err := generatePlaylist(recordings, mainPlaylist, outputDir, nil); err != nil {
		return fmt.Errorf("failed to generate main playlist: %v", err)
	}
	log.Printf("[POSTS] Generated main playlist with %d recordings", len(recordings))

	// Generate user-specific playlists
	for _, userPlaylist := range userPlaylists {
		log.Printf("[POSTS] Generating playlist for user %s: %s", userPlaylist.Username, userPlaylist.Filename)

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

// GenerateUnifiedFeed creates a shows.json combining recordings and posts
func GenerateUnifiedFeed(posts []Post, recordings []Recording, outputDir string) error {
	// Create a map of recording keys to posts for easy lookup
	recordingToPost := make(map[string]*Post)
	for i := range posts {
		if posts[i].Metadata.Recording != "" {
			log.Printf("[POSTS] Post '%s' (ID: %s) linked to recording: '%s'", posts[i].Title, posts[i].ID, posts[i].Metadata.Recording)
			recordingToPost[posts[i].Metadata.Recording] = &posts[i]
		}
	}
	log.Printf("[POSTS] Created map with %d post-to-recording associations", len(recordingToPost))

	// Track which posts have been used (linked to recordings)
	usedPosts := make(map[string]bool)

	var showItems []ShowItem

	// Add recordings (with or without posts)
	for _, rec := range recordings {
		// Parse date for sorting (use single-digit day format)
		dateTime, err := time.Parse("January 2, 2006", rec.Date)
		if err != nil {
			// If parse fails, use LastModified
			log.Printf("[POSTS] WARNING: Failed to parse date '%s' for recording %s: %v", rec.Date, rec.Key, err)
			dateTime = rec.LastModified
		}

		item := ShowItem{
			Title:     rec.DisplayName,
			Author:    rec.DJ,
			Date:      rec.Date,
			Timestamp: dateTime,
			Recording: &RecordingInfo{
				Key: rec.Key,
				URL: rec.URL,
			},
		}

		// Check if there's a post linked to this recording
		if post, exists := recordingToPost[rec.Key]; exists {
			log.Printf("[POSTS] ✓ Matched recording '%s' to post '%s'", rec.Key, post.Title)
			item.Post = &PostInfo{
				Slug:    post.Slug,
				Excerpt: post.Metadata.Excerpt,
			}
			item.Title = post.Title
			usedPosts[post.ID] = true
		} else {
			log.Printf("[POSTS] ✗ No post found for recording '%s'", rec.Key)
		}

		showItems = append(showItems, item)
	}

	// Add posts that aren't linked to recordings
	for _, post := range posts {
		if !usedPosts[post.ID] {
			item := ShowItem{
				Title:     post.Title,
				Author:    post.Author,
				Date:      post.CreatedAt.Format("January 2, 2006"),
				Timestamp: post.CreatedAt,
				Post: &PostInfo{
					Slug:    post.Slug,
					Excerpt: post.Metadata.Excerpt,
				},
			}
			showItems = append(showItems, item)
		}
	}

	// Sort by timestamp (newest first)
	sort.Slice(showItems, func(i, j int) bool {
		return showItems[i].Timestamp.After(showItems[j].Timestamp)
	})

	// Marshal to JSON
	data, err := json.MarshalIndent(showItems, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal shows.json: %v", err)
	}

	outputFile := filepath.Join(outputDir, "shows.json")
	if err := ioutil.WriteFile(outputFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write shows.json: %v", err)
	}

	log.Printf("[POSTS] Generated shows.json with %d items (%d recordings, %d standalone posts)",
		len(showItems), len(recordings), len(posts)-len(usedPosts))
	return nil
}

// cleanupOrphanedFiles removes HTML files that no longer correspond to published posts
func cleanupOrphanedFiles(posts []Post, outputDir string) error {
	patchDir := filepath.Join(outputDir, "patch")

	// Create a set of valid slugs
	validSlugs := make(map[string]bool)
	for _, post := range posts {
		validSlugs[post.Slug] = true
	}

	// List all HTML files in patch directory
	files, err := ioutil.ReadDir(patchDir)
	if err != nil {
		// If directory doesn't exist, nothing to clean up
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read patch directory: %v", err)
	}

	// Delete files that don't match any current post slug
	deletedCount := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filename := file.Name()
		// Only process .html files
		if len(filename) < 5 || filename[len(filename)-5:] != ".html" {
			continue
		}

		// Extract slug from filename (remove .html extension)
		slug := filename[:len(filename)-5]

		// If this slug doesn't match any current post, delete it
		if !validSlugs[slug] {
			filePath := filepath.Join(patchDir, filename)
			if err := os.Remove(filePath); err != nil {
				log.Printf("[POSTS] WARNING: Failed to delete orphaned file %s: %v", filename, err)
			} else {
				log.Printf("[POSTS] Deleted orphaned file: %s", filename)
				deletedCount++
			}
		}
	}

	if deletedCount > 0 {
		log.Printf("[POSTS] Cleaned up %d orphaned file(s)", deletedCount)
	}

	return nil
}

// Run executes the full post generation workflow
func Run(config Config) error {
	log.Printf("[POSTS] Starting post generation process")

	// List all published posts
	posts, err := ListPosts(config.BucketClient)
	if err != nil {
		return fmt.Errorf("failed to list posts: %v", err)
	}

	// Generate HTML for each post
	for _, post := range posts {
		if err := GeneratePostHTML(post, config.OutputDir); err != nil {
			log.Printf("[POSTS] WARNING: Failed to generate HTML for %s: %v", post.Title, err)
			continue
		}
	}

	// Clean up orphaned HTML files (from renamed, unpublished, or deleted posts)
	// This runs even if there are no published posts to clean up old files
	if err := cleanupOrphanedFiles(posts, config.OutputDir); err != nil {
		log.Printf("[POSTS] WARNING: Failed to clean up orphaned files: %v", err)
		// Don't fail the whole process if cleanup fails
	}

	// Fetch recordings from S3 for playlist and feed generation
	log.Printf("[POSTS] Fetching recordings for codegen...")
	recordings, err := fetchRecordingsFromS3(config.BucketClient)
	if err != nil {
		log.Printf("[POSTS] WARNING: Failed to fetch recordings from S3: %v", err)
		log.Printf("[POSTS] Skipping playlist and feed generation")
		log.Printf("[POSTS] Post generation complete: %d posts", len(posts))
		return nil
	}

	// Generate playlists from recordings
	if err := GeneratePlaylists(recordings, config.OutputDir); err != nil {
		return fmt.Errorf("failed to generate playlists: %v", err)
	}

	// Generate unified shows.json combining recordings and posts
	log.Printf("[POSTS] Generating unified shows feed...")
	if err := GenerateUnifiedFeed(posts, recordings, config.OutputDir); err != nil {
		return fmt.Errorf("failed to generate shows.json: %v", err)
	}

	log.Printf("[POSTS] Code generation complete: %d posts, %d recordings", len(posts), len(recordings))
	return nil
}
