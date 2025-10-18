package posts

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
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
            <div class="post-container" style="margin-bottom: 24px; background: var(--daorange); color: white;">
                <h3 style="margin: 0 0 16px 0; font-family: 'Cooper Black Regular', monospace;">🎵 Listen to this show</h3>
                <audio controls style="width: 100%%; border-radius: 8px;">
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
    <title>%s - cabbage.town</title>
    <link rel="icon" href="../icon.ico" type="image/x-icon">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link rel="stylesheet" href="../reset.css">
    <style>
        @font-face {
            font-family: 'Cooper Black Regular';
            font-style: normal;
            font-weight: normal;
            src: local('Cooper Black Regular'), url('../COOPBL.woff') format('woff');
        }

        :root {
            --dagreen: rgb(36, 221, 35);
            --dayellow: rgb(255, 237, 182);
            --daorange: rgb(250, 134, 37);
        }

        body {
            background: var(--dayellow);
            color: black;
            font-family: 'Courier New', Courier, monospace;
            margin: 0;
            padding: 8px;
        }

        .main {
            display: flex;
            flex-direction: column;
            align-items: center;
        }

        .content {
            display: flex;
            flex-direction: column;
            max-width: 700px;
            gap: 16px;
            padding: 16px;
            width: 100%%;
        }

        .post-container {
            background: white;
            border-radius: 32px;
            padding: 32px;
        }

        .post-header {
            margin-bottom: 24px;
        }

        .post-title {
            font-family: 'Cooper Black Regular', monospace;
            font-size: 2em;
            margin-bottom: 8px;
            color: var(--daorange);
        }

        .post-meta {
            color: #666;
            font-size: 0.9em;
        }

        .post-content {
            line-height: 1.6;
        }

        .post-content h1,
        .post-content h2,
        .post-content h3 {
            font-family: 'Cooper Black Regular', monospace;
            color: var(--daorange);
            margin-top: 24px;
            margin-bottom: 12px;
        }

        .post-content h1 { font-size: 1.8em; }
        .post-content h2 { font-size: 1.5em; }
        .post-content h3 { font-size: 1.2em; }

        .post-content p {
            margin-bottom: 16px;
        }

        .post-content a {
            color: blue;
            text-decoration: underline;
        }

        .post-content img {
            max-width: 100%%;
            height: auto;
            border-radius: 8px;
            margin: 16px 0;
        }

        .post-content code {
            background: #f4f4f4;
            padding: 2px 6px;
            border-radius: 3px;
            font-family: 'Courier New', Courier, monospace;
        }

        .post-content pre {
            background: #f4f4f4;
            padding: 16px;
            border-radius: 8px;
            overflow-x: auto;
        }

        .post-content pre code {
            background: none;
            padding: 0;
        }

        .post-content ul, .post-content ol {
            margin-bottom: 16px;
            padding-left: 32px;
        }

        .post-content li {
            margin-bottom: 8px;
        }

        .back-link {
            color: blue;
            text-decoration: none;
            font-size: 0.9em;
        }

        .back-link:hover {
            text-decoration: underline;
        }

        .syne-mono-regular {
            font-family: "Cooper Black Regular", monospace;
        }
    </style>
</head>
<body>
    <div class="main">
        <a href="/">
            <img src="../the-cabbage.png" style="width: 80px; height: auto" alt="cabbage">
        </a>
        <a href="/" style="text-decoration: none; color: black;">
            <h2 class="syne-mono-regular">cabbage.town</h2>
        </a>

        <div class="content">
            <a href="/" class="back-link">back to the patch</a>
            %s
            <div class="post-container">
                <div class="post-header">
                    <h1 class="post-title">%s</h1>
                    <div class="post-meta">
                        By <strong>%s</strong> · %s
                    </div>
                </div>
                
                <div class="post-content">
                    %s
                </div>
            </div>

            <a href="/" class="back-link">back to the patch</a>
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

// parseRSSFeed reads and parses the feed.xml to get recordings
func parseRSSFeed(outputDir string) ([]Recording, error) {
	feedPath := filepath.Join(outputDir, "feed.xml")
	data, err := ioutil.ReadFile(feedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read feed.xml: %v", err)
	}

	// Simple RSS structure for parsing
	type RSS struct {
		XMLName xml.Name `xml:"rss"`
		Channel struct {
			Items []struct {
				Title   string `xml:"title"`
				Link    string `xml:"link"`
				PubDate string `xml:"pubDate"`
				Author  string `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd author"`
				GUID    string `xml:"guid"`
			} `xml:"item"`
		} `xml:"channel"`
	}

	var rss RSS
	if err := xml.Unmarshal(data, &rss); err != nil {
		return nil, fmt.Errorf("failed to parse feed.xml: %v", err)
	}

	var recordings []Recording
	for _, item := range rss.Channel.Items {
		// Parse pubDate
		pubDate, err := time.Parse(time.RFC1123Z, item.PubDate)
		if err != nil {
			log.Printf("[POSTS] WARNING: Failed to parse date %s: %v", item.PubDate, err)
			pubDate = time.Now()
		}

		// Extract key from URL (GUID is the full URL)
		url := item.GUID
		key := ""
		if strings.Contains(url, "cabbagetown.nyc3.digitaloceanspaces.com/") {
			parts := strings.Split(url, "cabbagetown.nyc3.digitaloceanspaces.com/")
			if len(parts) > 1 {
				key = parts[1]
			}
		}

		recordings = append(recordings, Recording{
			URL:          url,
			Key:          key,
			DJ:           item.Author,
			Show:         item.Title,
			Date:         pubDate.Format("January 2, 2006"),
			LastModified: pubDate,
			DisplayName:  item.Title,
		})
	}

	return recordings, nil
}

// GenerateUnifiedFeed creates a shows.json combining recordings and posts
func GenerateUnifiedFeed(posts []Post, recordings []Recording, outputDir string) error {
	// Create a map of recording keys to posts for easy lookup
	recordingToPost := make(map[string]*Post)
	for i := range posts {
		if posts[i].Metadata.Recording != "" {
			recordingToPost[posts[i].Metadata.Recording] = &posts[i]
		}
	}

	// Track which posts have been used (linked to recordings)
	usedPosts := make(map[string]bool)

	var showItems []ShowItem

	// Add recordings (with or without posts)
	for _, rec := range recordings {
		// Parse date for sorting
		dateTime, err := time.Parse("January 02, 2006", rec.Date)
		if err != nil {
			// If parse fails, use LastModified
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
			item.Post = &PostInfo{
				Slug:    post.Slug,
				Excerpt: post.Metadata.Excerpt,
			}
			usedPosts[post.ID] = true
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

	// Generate unified shows.json combining recordings and posts
	log.Printf("[POSTS] Generating unified shows feed...")
	recordings, err := parseRSSFeed(config.OutputDir)
	if err != nil {
		log.Printf("[POSTS] WARNING: Failed to parse RSS feed: %v", err)
		log.Printf("[POSTS] Skipping unified feed generation")
	} else {
		if err := GenerateUnifiedFeed(posts, recordings, config.OutputDir); err != nil {
			return fmt.Errorf("failed to generate shows.json: %v", err)
		}
	}

	log.Printf("[POSTS] Post generation complete: %d posts", len(posts))
	return nil
}
