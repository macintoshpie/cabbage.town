package trellis

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Config struct {
	BucketURL  string
	ListURL    string
	OutputDir  string
	OutputFile string
	RSSFile    string
}

type ListBucketResult struct {
	Contents []Content `xml:"Contents"`
}

type Content struct {
	Key string `xml:"Key"`
}

type Recording struct {
	URL  string
	DJ   string
	Show string
	Date string
}

type RSS struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel Channel  `xml:"channel"`
}

type Channel struct {
	Title          string   `xml:"title"`
	Link           string   `xml:"link"`
	AtomLink       AtomLink `xml:"atom:link"`
	Description    string   `xml:"description"`
	Language       string   `xml:"language"`
	PubDate        string   `xml:"pubDate"`
	LastBuildDate  string   `xml:"lastBuildDate"`
	Generator      string   `xml:"generator"`
	Author         string   `xml:"itunes:author"`
	Owner          Owner    `xml:"itunes:owner"`
	Image          Image    `xml:"itunes:image"`
	ItunesCategory Category `xml:"itunes:category"`
	Explicit       string   `xml:"itunes:explicit"`
	Type           string   `xml:"itunes:type"`
	Items          []Item   `xml:"item"`
}

type AtomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type Owner struct {
	Name  string `xml:"itunes:name"`
	Email string `xml:"itunes:email"`
}

type Image struct {
	Href string `xml:"href,attr"`
}

type Category struct {
	Text        string       `xml:"text,attr"`
	Subcategory *Subcategory `xml:"itunes:category,omitempty"`
}

type Subcategory struct {
	Text string `xml:"text,attr"`
}

type Item struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	PubDate     string    `xml:"pubDate"`
	GUID        string    `xml:"guid"`
	Duration    string    `xml:"itunes:duration"`
	Explicit    string    `xml:"itunes:explicit"`
	Enclosure   Enclosure `xml:"enclosure"`
}

type Enclosure struct {
	URL    string `xml:"url,attr"`
	Length string `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

func Run(config Config) error {
	fmt.Println("🌱 tending the patch...")

	allRecordings, err := listRecordings(config)
	if err != nil {
		return fmt.Errorf("failed to list recordings: %v", err)
	}

	recordings := filterUnavailableRecordings(allRecordings)

	err = updatePlaylist(recordings, config)
	if err != nil {
		return fmt.Errorf("failed to update playlist: %v", err)
	}

	err = updateRssFeed(recordings, config)
	if err != nil {
		return fmt.Errorf("failed to update RSS feed: %v", err)
	}

	fmt.Println("Playlist update complete.")
	return nil
}

func listRecordings(config Config) ([]Recording, error) {
	// Fetch list of recordings
	resp, err := http.Get(config.ListURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch list of recordings: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Parse XML response
	var result ListBucketResult
	if err := xml.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %v", err)
	}

	var recordings []Recording
	for _, content := range result.Contents {
		if content.Key != "" && content.Key[len(content.Key)-4:] == ".mp3" {
			fullURL := config.BucketURL + "/" + content.Key
			recording, err := parseRecordingInfo(fullURL)
			if err != nil {
				fmt.Printf("Failed to parse recording info for %s: %v\n", fullURL, err)
				continue
			}
			recordings = append(recordings, recording)
		}
	}

	// Sort recordings by date in descending order
	sort.Slice(recordings, func(i, j int) bool {
		dateI, _ := time.Parse("January 02, 2006", recordings[i].Date)
		dateJ, _ := time.Parse("January 02, 2006", recordings[j].Date)
		return dateI.After(dateJ)
	})

	return recordings, nil
}

func filterUnavailableRecordings(recordings []Recording) []Recording {
	var availableRecordings []Recording
	for _, recording := range recordings {
		resp, err := http.Head(recording.URL)
		if err != nil {
			fmt.Printf("Failed to access %s: %v\n", recording.URL, err)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			availableRecordings = append(availableRecordings, recording)
		} else {
			fmt.Printf("Failed to access %s\n", recording.URL)
		}
	}

	return availableRecordings
}

func updatePlaylist(recordings []Recording, config Config) error {
	// Create directory for output file
	if err := os.MkdirAll(config.OutputDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Initialize playlist file
	outputFilePath := filepath.Join(config.OutputDir, config.OutputFile)
	if err := ioutil.WriteFile(outputFilePath, []byte("#EXTM3U\n"), 0644); err != nil {
		return fmt.Errorf("failed to initialize playlist file: %v", err)
	}

	// Update playlist with sorted recordings
	for _, recording := range recordings {
		entry := fmt.Sprintf("#EXTINF:-1,%s - %s (%s)\n%s\n", recording.Show, recording.DJ, recording.Date, recording.URL)
		if err := appendToFile(outputFilePath, entry); err != nil {
			return fmt.Errorf("failed to append to playlist file: %v", err)
		}
	}

	return nil
}

func updateRssFeed(recordings []Recording, config Config) error {
	now := time.Now().Format(time.RFC1123Z)
	feedURL := "https://cabbage.town/feed.xml"

	rss := RSS{
		Version: "2.0",
		Channel: Channel{
			Title: "Cabbage Town Radio",
			Link:  "https://cabbage.town",
			AtomLink: AtomLink{
				Href: feedURL,
				Rel:  "self",
				Type: "application/rss+xml",
			},
			Description:   "Live recordings from Cabbage Town Radio",
			Language:      "en-us",
			PubDate:       now,
			LastBuildDate: now,
			Generator:     "Cabbage Town Radio Feed Generator",
			Author:        "Cabbage Town Radio",
			Owner: Owner{
				Name:  "Cabbage Town Radio",
				Email: "radio@cabbage.town",
			},
			Image: Image{
				Href: "https://cabbage.town/the-cabbage.png",
			},
			ItunesCategory: Category{
				Text: "Music",
				Subcategory: &Subcategory{
					Text: "Music Commentary",
				},
			},
			Explicit: "false",
			Type:     "episodic",
			Items:    make([]Item, 0, len(recordings)),
		},
	}

	for _, recording := range recordings {
		date, err := time.Parse("January 02, 2006", recording.Date)
		if err != nil {
			fmt.Printf("Failed to parse date for RSS item: %v\n", err)
			continue
		}

		item := Item{
			Title: fmt.Sprintf("%s with %s", recording.Show, recording.DJ),
			Link:  recording.URL,
			Description: fmt.Sprintf("Episode of %s with %s, recorded on %s",
				recording.Show, recording.DJ, recording.Date),
			PubDate:  date.Format(time.RFC1123Z),
			GUID:     recording.URL,
			Duration: "3600",
			Explicit: "false",
			Enclosure: Enclosure{
				URL:    recording.URL,
				Type:   "audio/mpeg",
				Length: "0",
			},
		}
		rss.Channel.Items = append(rss.Channel.Items, item)
	}

	// Marshal to XML with proper namespaces
	output, err := xml.MarshalIndent(rss, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal RSS feed: %v", err)
	}

	// Add XML header and namespaces
	xmlData := []byte(xml.Header +
		`<rss version="2.0" 
			xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd" 
			xmlns:content="http://purl.org/rss/1.0/modules/content/"
			xmlns:atom="http://www.w3.org/2005/Atom">` +
		string(output[len("<rss version=\"2.0\">"):]))

	// Write to file
	rssFilePath := filepath.Join(config.OutputDir, config.RSSFile)
	err = ioutil.WriteFile(rssFilePath, xmlData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write RSS feed file: %v", err)
	}

	return nil
}

func appendToFile(filename, text string) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(text); err != nil {
		return err
	}
	return nil
}

func parseRecordingInfo(url string) (Recording, error) {
	// Example URL: https://cabbagetown.nyc3.digitaloceanspaces.com/recordings/brennan/stream_20250626-204143.mp3
	// Extract the relevant parts from the URL
	parts := strings.Split(url, "/")
	if len(parts) < 5 {
		return Recording{}, fmt.Errorf("invalid URL format")
	}

	bucketFolder := parts[4]
	show, dj, err := getShowName(bucketFolder)
	if err != nil {
		return Recording{}, err
	}

	filename := parts[len(parts)-1]
	// Extract date from filename by finding the last occurrence of YYYYMMDD pattern
	dateStr := regexp.MustCompile(`(\d{8})-\d{6}`).FindString(filename)[:8] // Extract date from filename

	// Parse the date string
	date, err := time.Parse("20060102", dateStr)
	if err != nil {
		return Recording{}, fmt.Errorf("failed to parse date: %v", err)
	}

	// Format the date as desired, e.g., "January 02, 2006"
	formattedDate := date.Format("January 02, 2006")

	return Recording{
		URL:  url,
		DJ:   dj,
		Show: show,
		Date: formattedDate,
	}, nil
}

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
