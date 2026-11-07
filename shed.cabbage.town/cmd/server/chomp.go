package main

// Chomp & Stomp 2026 selfie booth.
//
//   POST /api/chomp            multipart: image, team (fire|water|grass), nick (optional)
//                              -> stores chomp/<id>.jpg and chomp/<id>.json {status:"queued", ...}
//                              -> {"id": "<id>"}
//   POST /api/chomp/result     bearer CHOMP_WORKER_TOKEN; multipart: id, status (generating|done|failed),
//                              name (optional), sprite (png, optional)
//                              -> writes chomp/<id>.sprite.png and rewrites chomp/<id>.json
//
// The phone and the projector read chomp/* straight from the bucket (public-read), so nothing
// here needs a GET route. The worker that calls /api/chomp/result lives on the projector laptop.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
)

type chompRecord struct {
	ID      string `json:"id"`
	Team    string `json:"team"`
	Nick    string `json:"nick,omitempty"`
	Name    string `json:"name,omitempty"`  // the Pokémon's name, set by the worker
	Style   string `json:"style,omitempty"` // art style the worker picked (standard, chonky, final, baby, pixel)
	Status  string `json:"status"`          // queued | generating | done | failed
	Selfie  string `json:"selfie"`
	Sprite  string `json:"sprite,omitempty"`
	Created string `json:"created"`
	Updated string `json:"updated"`
}

var chompIDRe = regexp.MustCompile(`^[a-z0-9-]{8,48}$`)

func chompPutJSON(rec *chompRecord) error {
	rec.Updated = time.Now().UTC().Format(time.RFC3339)
	body, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return bucketClient.PutObjectWithMetadata("chomp/"+rec.ID+".json", strings.NewReader(string(body)), "application/json", nil, "public-read")
}

func chompGetJSON(id string) (*chompRecord, error) {
	out, err := bucketClient.GetObject("chomp/" + id + ".json")
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	var rec chompRecord
	if err := json.NewDecoder(out.Body).Decode(&rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func chompUploadHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}
	team := strings.ToLower(r.FormValue("team"))
	if team != "fire" && team != "water" && team != "grass" {
		http.Error(w, "team must be fire, water or grass", http.StatusBadRequest)
		return
	}
	nick := strings.TrimSpace(r.FormValue("nick"))
	if len(nick) > 24 {
		nick = nick[:24]
	}
	nick = regexp.MustCompile(`[^\p{L}\p{N} _.'!?-]`).ReplaceAllString(nick, "")

	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Failed to get image from request", http.StatusBadRequest)
		return
	}
	defer file.Close()
	contentType := header.Header.Get("Content-Type")
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" {
		http.Error(w, "image must be jpeg, png or webp", http.StatusBadRequest)
		return
	}

	b := make([]byte, 6)
	rand.Read(b)
	id := time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b)
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}[contentType]
	selfieKey := "chomp/" + id + ext

	meta := map[string]*string{"Team": aws.String(team), "Nick": aws.String(nick)}
	if err := bucketClient.PutObjectWithMetadata(selfieKey, file, contentType, meta, "public-read"); err != nil {
		log.Printf("chomp: upload failed: %v", err)
		http.Error(w, "Failed to upload image", http.StatusInternalServerError)
		return
	}
	rec := &chompRecord{ID: id, Team: team, Nick: nick, Status: "queued", Selfie: selfieKey, Created: time.Now().UTC().Format(time.RFC3339)}
	if err := chompPutJSON(rec); err != nil {
		log.Printf("chomp: record failed: %v", err)
		http.Error(w, "Failed to write record", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id, "selfie": fmt.Sprintf("https://%s.nyc3.digitaloceanspaces.com/%s", bucketClient.Bucket, selfieKey)})
}

func chompResultHandler(w http.ResponseWriter, r *http.Request) {
	token := os.Getenv("CHOMP_WORKER_TOKEN")
	if token == "" || r.Header.Get("Authorization") != "Bearer "+token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}
	id := r.FormValue("id")
	if !chompIDRe.MatchString(id) {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	status := r.FormValue("status")
	if status != "generating" && status != "done" && status != "failed" {
		http.Error(w, "bad status", http.StatusBadRequest)
		return
	}
	rec, err := chompGetJSON(id)
	if err != nil {
		http.Error(w, "unknown id", http.StatusNotFound)
		return
	}
	if name := strings.TrimSpace(r.FormValue("name")); name != "" && len(name) <= 24 {
		rec.Name = name
	}
	if style := strings.TrimSpace(r.FormValue("style")); style != "" && len(style) <= 16 {
		rec.Style = style
	}
	if file, _, err := r.FormFile("sprite"); err == nil {
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil || len(data) == 0 {
			http.Error(w, "bad sprite", http.StatusBadRequest)
			return
		}
		key := "chomp/" + id + ".sprite.png"
		if err := bucketClient.PutObjectWithMetadata(key, strings.NewReader(string(data)), "image/png", nil, "public-read"); err != nil {
			log.Printf("chomp: sprite upload failed: %v", err)
			http.Error(w, "Failed to upload sprite", http.StatusInternalServerError)
			return
		}
		rec.Sprite = key
	}
	rec.Status = status
	if err := chompPutJSON(rec); err != nil {
		http.Error(w, "Failed to write record", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rec)
}
