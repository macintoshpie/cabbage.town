package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/joho/godotenv"
	"golang.org/x/time/rate"

	"cabbage.town/shed.cabbage.town/pkg/bucket"
)

const (
	sessionName   = "cabbage-session"
	userFile      = "shed/users.json"
	maxUploadSize = 500 * 1024 * 1024 // 500MB
	bucketName    = "cabbagetown"     // Add bucket name constant
)

type UserStore struct {
	Users map[string]bucket.User `json:"users"`
	mu    sync.RWMutex
}

var (
	templates    = parseTemplates()
	store        *sessions.CookieStore
	bucketClient *bucket.Client
	users        = &UserStore{
		Users: make(map[string]bucket.User),
	}
	userRefreshTicker *time.Ticker
	userRefreshDone   chan bool
)

func parseTemplates() *template.Template {
	log.Printf("[TEMPLATE] Starting template parsing")

	// Parse all templates
	tmpl, err := template.ParseFiles(
		"templates/login.html",
		"templates/files.html",
		"templates/admin_users.html",
		"templates/upload.html",
		"templates/posts_list.html",
		"templates/post_editor.html",
		"templates/post_view.html",
	)
	if err != nil {
		log.Fatalf("[TEMPLATE] Failed to parse templates: %v", err)
	}

	// Log all defined templates
	for _, t := range tmpl.Templates() {
		log.Printf("[TEMPLATE] Found template: %s", t.Name())
	}

	return tmpl
}

// Response types
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type AdminResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ToggleAccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type AddUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"isAdmin"`
}

type ToggleAdminRequest struct {
	IsAdmin bool `json:"isAdmin"`
}

type FileInfo struct {
	Key          string             `json:"key"`
	IsPublic     bool               `json:"isPublic"`
	Owner        string             `json:"owner"`
	SizeMB       float64            `json:"sizeMB"` // Changed from SizeBytes
	LastModified time.Time          `json:"lastModified"`
	Metadata     map[string]*string `json:"metadata"`
	PostID       string             `json:"postId,omitempty"`   // ID of associated post, if any
	PostSlug     string             `json:"postSlug,omitempty"` // Slug of associated post, if any
}

// Add this helper method
func (f FileInfo) DisplayName() string {
	if displayName, ok := f.Metadata["Display-Name"]; ok && displayName != nil {
		return *displayName
	}
	// Extract filename from key as fallback
	parts := strings.Split(f.Key, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return f.Key
}

type ToggleAccessRequest struct {
	Key        string `json:"key"`
	MakePublic bool   `json:"makePublic"`
}

// Add this type near other type definitions
type FilePermissionCheck struct {
	IsAdmin bool
	Owner   string
	Key     string
}

type RenameFileRequest struct {
	Key         string `json:"key"`
	DisplayName string `json:"displayName"`
}

// Post types
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

type PostListItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Slug      string    `json:"slug"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Published bool      `json:"published"`
	Excerpt   string    `json:"excerpt"`
}

type CreatePostRequest struct {
	Title     string `json:"title"`
	Author    string `json:"author"`
	Markdown  string `json:"markdown"`
	Published bool   `json:"published"`
	Recording string `json:"recording"`
}

type UpdatePostRequest struct {
	Title     string `json:"title"`
	Author    string `json:"author"`
	Markdown  string `json:"markdown"`
	Published bool   `json:"published"`
	Recording string `json:"recording"`
}

// Auth handlers
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var loginReq LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
		log.Printf("Failed to decode login request: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	log.Printf("Login attempt for user: %s", loginReq.Username)
	if isValidCredentials(loginReq.Username, loginReq.Password) {
		session, err := store.Get(r, sessionName)
		if err != nil {
			log.Printf("Failed to get session: %v", err)
			http.Error(w, "Session error", http.StatusInternalServerError)
			return
		}

		session.Values["authenticated"] = true
		session.Values["username"] = loginReq.Username
		session.Options.MaxAge = 86400 * 7
		session.Options.Path = "/"
		session.Options.HttpOnly = true

		if err := session.Save(r, w); err != nil {
			log.Printf("Failed to save session: %v", err)
			http.Error(w, "Session error", http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(LoginResponse{
			Success: true,
			Message: "Login successful",
		})
		return
	}

	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(LoginResponse{
		Success: false,
		Message: "Invalid credentials",
	})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	session, err := store.Get(r, sessionName)
	if err != nil {
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	session.Options.MaxAge = -1
	session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// File handlers
func listFilesHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Determine prefix based on admin status
	var prefix string
	if isAdmin(username) {
		prefix = "recordings/" // List all recordings for admins
	} else {
		prefix = fmt.Sprintf("recordings/%s/", username) // List only user's recordings
	}

	objects, err := bucketClient.ListObjects(prefix)
	if err != nil {
		log.Printf("Error listing files: %v", err)
		http.Error(w, "Failed to list files", http.StatusInternalServerError)
		return
	}

	var files []FileInfo
	for _, obj := range objects {
		// Skip directories
		if strings.HasSuffix(*obj.Key, "/") {
			continue
		}

		// Get ACL to check if public
		aclOutput, err := bucketClient.GetObjectACL(*obj.Key)
		if err != nil {
			log.Printf("Error getting ACL for %s: %v", *obj.Key, err)
			continue
		}

		isPublic := false
		for _, grant := range aclOutput.Grants {
			if grant.Grantee.URI != nil && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
				isPublic = true
				break
			}
		}

		// Get object metadata
		headOutput, err := bucketClient.HeadObject(*obj.Key)
		if err != nil {
			log.Printf("Error getting object metadata for %s: %v", *obj.Key, err)
			continue
		}

		// Extract owner username from the path
		pathParts := strings.Split(*obj.Key, "/")
		var owner string
		if len(pathParts) >= 2 {
			owner = pathParts[1] // recordings/<username>/file.mp3
		}

		// Convert size to MB with 2 decimal precision
		sizeMB := float64(*headOutput.ContentLength) / 1048576.0 // 1024 * 1024

		files = append(files, FileInfo{
			Key:          *obj.Key,
			IsPublic:     isPublic,
			Owner:        owner,
			SizeMB:       sizeMB,
			LastModified: *headOutput.LastModified,
			Metadata:     headOutput.Metadata,
		})
	}

	// Fetch all posts and create a map of recording key -> post
	postObjects, err := bucketClient.ListObjects("posts/")
	if err == nil {
		recordingToPost := make(map[string]*Post)

		for _, postObj := range postObjects {
			if postObj.Key == nil || !strings.HasSuffix(*postObj.Key, ".json") {
				continue
			}

			// Fetch the post
			output, err := bucketClient.GetObject(*postObj.Key)
			if err != nil {
				continue
			}
			defer output.Body.Close()

			data, err := ioutil.ReadAll(output.Body)
			if err != nil {
				continue
			}

			var post Post
			if err := json.Unmarshal(data, &post); err != nil {
				continue
			}

			// Only map non-deleted posts with recordings
			if post.DeletedAt == nil && post.Metadata.Recording != "" {
				recordingToPost[post.Metadata.Recording] = &post
			}
		}

		// Update files with post information
		for i := range files {
			if post, exists := recordingToPost[files[i].Key]; exists {
				files[i].PostID = post.ID
				files[i].PostSlug = post.Slug
			}
		}
	}

	data := struct {
		Files    []FileInfo
		IsAdmin  bool
		Username string
	}{
		Files:    files,
		IsAdmin:  isAdmin(username),
		Username: username,
	}

	if err := templates.ExecuteTemplate(w, "files.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Limit request size
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		http.Error(w, "File too large. Maximum size is 500MB.", http.StatusBadRequest)
		return
	}

	// Get form values
	dateStr := r.FormValue("date")

	// Parse the date
	date, err := time.Parse("2006-01-02T15:04", dateStr)
	if err != nil {
		http.Error(w, "Invalid date format", http.StatusBadRequest)
		return
	}

	// Get the file
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to get file from request", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Generate filename in the format: stream_YYYYMMDD-HHMMSS.ext
	ext := filepath.Ext(header.Filename)
	// Sanitize extension
	ext = regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(ext, "")
	if ext == "" {
		http.Error(w, "Invalid file extension", http.StatusBadRequest)
		return
	}
	ext = "." + ext

	filename := fmt.Sprintf("stream_%s%s", date.Format("20060102-150405"), ext)
	key := fmt.Sprintf("recordings/%s/%s", username, filename)

	// Validate the generated key
	if err := sanitizeAndValidateKey(key); err != nil {
		log.Printf("Generated invalid file path: %s - Error: %v", key, err)
		http.Error(w, "Invalid file path generated", http.StatusInternalServerError)
		return
	}

	// Upload the file
	if err := bucketClient.PutObjectStreaming(key, file, header.Header.Get("Content-Type")); err != nil {
		log.Printf("Error uploading file: %v", err)
		http.Error(w, "Failed to upload file", http.StatusInternalServerError)
		return
	}

	// Redirect back to files page
	http.Redirect(w, r, "/files", http.StatusSeeOther)
}

func uploadPageHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[UPLOAD] Request to %s", r.URL.Path)
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		log.Printf("[UPLOAD] No username in session")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data := struct {
		Username string
		IsAdmin  bool
	}{
		Username: username,
		IsAdmin:  isAdmin(username),
	}

	if err := templates.ExecuteTemplate(w, "upload.html", data); err != nil {
		log.Printf("[UPLOAD] Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Add this function near other helper functions
func checkFilePermissions(check FilePermissionCheck) error {
	if check.IsAdmin {
		return nil // Admins have full access
	}

	// Extract owner from path
	parts := strings.Split(check.Key, "/")
	if len(parts) != 3 {
		return fmt.Errorf("invalid path structure")
	}
	fileOwner := parts[1]

	// If user is not admin and not the owner, they need explicit permission
	if fileOwner != check.Owner {
		return fmt.Errorf("unauthorized access")
	}

	return nil
}

// Update viewFileHandler to use atomic permission check
func viewFileHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	key := vars["key"]

	if err := sanitizeAndValidateKey(key); err != nil {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	// Perform atomic permission check
	permCheck := FilePermissionCheck{
		IsAdmin: isAdmin(username),
		Owner:   username,
		Key:     key,
	}

	if err := checkFilePermissions(permCheck); err != nil {
		// If not admin or owner, check if file is public
		aclOutput, aclErr := bucketClient.GetObjectACL(key)
		if aclErr != nil {
			log.Printf("Error getting ACL for %s: %v", key, aclErr)
			http.Error(w, "Failed to check file access", http.StatusInternalServerError)
			return
		}

		isPublic := false
		for _, grant := range aclOutput.Grants {
			if grant.Grantee.URI != nil && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
				isPublic = true
				break
			}
		}

		if !isPublic {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Generate presigned URL (valid for 1 hour)
	url, err := bucketClient.GetPresignedURL(key, time.Hour)
	if err != nil {
		log.Printf("Error generating presigned URL: %v", err)
		http.Error(w, "Failed to generate file URL", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// Update toggleAccessHandler to use atomic permission check
func toggleAccessHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ToggleAccessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := sanitizeAndValidateKey(req.Key); err != nil {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	// Perform atomic permission check
	permCheck := FilePermissionCheck{
		IsAdmin: isAdmin(username),
		Owner:   username,
		Key:     req.Key,
	}

	if err := checkFilePermissions(permCheck); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	acl := "private"
	if req.MakePublic {
		acl = "public-read"
	}

	// Get existing metadata to preserve it while adding privacy flags
	headOutput, err := bucketClient.HeadObject(req.Key)
	if err != nil {
		log.Printf("Error getting object metadata: %v", err)
		json.NewEncoder(w).Encode(ToggleAccessResponse{
			Success: false,
			Message: "Failed to get file metadata",
		})
		return
	}

	// Merge existing metadata with privacy updates
	mergedMetadata := make(map[string]*string)
	for k, v := range headOutput.Metadata {
		mergedMetadata[k] = v
	}
	mergedMetadata["Manually-Privated"] = aws.String(fmt.Sprintf("%v", !req.MakePublic))
	mergedMetadata["Privacy-Timestamp"] = aws.String(time.Now().UTC().Format(time.RFC3339))

	// Update object with merged metadata and new ACL
	_, err = bucketClient.CopyObject(&s3.CopyObjectInput{
		Bucket:            aws.String(bucketClient.Bucket),
		CopySource:        aws.String(fmt.Sprintf("%s/%s", bucketClient.Bucket, req.Key)),
		Key:               aws.String(req.Key),
		MetadataDirective: aws.String("REPLACE"),
		ACL:               aws.String(acl),
		Metadata:          mergedMetadata,
	})
	if err != nil {
		log.Printf("Error updating object: %v", err)
		json.NewEncoder(w).Encode(ToggleAccessResponse{
			Success: false,
			Message: "Failed to update file access",
		})
		return
	}

	json.NewEncoder(w).Encode(ToggleAccessResponse{
		Success: true,
		Message: "File access updated successfully",
	})
}

// Add this near other file handlers
func renameFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req RenameFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := sanitizeAndValidateKey(req.Key); err != nil {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	// Perform atomic permission check
	permCheck := FilePermissionCheck{
		IsAdmin: isAdmin(username),
		Owner:   username,
		Key:     req.Key,
	}

	if err := checkFilePermissions(permCheck); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get existing metadata
	headOutput, err := bucketClient.HeadObject(req.Key)
	if err != nil {
		log.Printf("Error getting object metadata: %v", err)
		http.Error(w, "Failed to get file metadata", http.StatusInternalServerError)
		return
	}

	// Get existing ACL
	aclOutput, err := bucketClient.GetObjectACL(req.Key)
	if err != nil {
		log.Printf("Error getting ACL for %s: %v", req.Key, err)
		http.Error(w, "Failed to get file ACL", http.StatusInternalServerError)
		return
	}

	// Determine if file is public
	acl := "private"
	for _, grant := range aclOutput.Grants {
		if grant.Grantee.URI != nil && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
			acl = "public-read"
			break
		}
	}

	// Merge existing metadata with display name
	mergedMetadata := make(map[string]*string)
	for k, v := range headOutput.Metadata {
		mergedMetadata[k] = v
	}
	mergedMetadata["Display-Name"] = aws.String(req.DisplayName)
	mergedMetadata["Display-Name-Timestamp"] = aws.String(time.Now().UTC().Format(time.RFC3339))

	// Update object with merged metadata and preserve existing ACL
	_, err = bucketClient.CopyObject(&s3.CopyObjectInput{
		Bucket:            aws.String(bucketClient.Bucket),
		CopySource:        aws.String(fmt.Sprintf("%s/%s", bucketClient.Bucket, req.Key)),
		Key:               aws.String(req.Key),
		MetadataDirective: aws.String("REPLACE"),
		ACL:               aws.String(acl),
		Metadata:          mergedMetadata,
	})
	if err != nil {
		log.Printf("Error updating object: %v", err)
		http.Error(w, "Failed to update file metadata", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(AdminResponse{
		Success: true,
		Message: "File renamed successfully",
	})
}

// Admin handlers
func adminUsersPageHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok || !isAdmin(username) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	users.mu.RLock()
	data := struct {
		Users    map[string]bucket.User
		Username string
		IsAdmin  bool
	}{
		Users:    users.Users,
		Username: username,
		IsAdmin:  true,
	}
	users.mu.RUnlock()

	if err := templates.ExecuteTemplate(w, "admin_users.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func addUserHandler(w http.ResponseWriter, r *http.Request) {
	var req AddUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Check if user already exists
	users.mu.RLock()
	_, exists := users.Users[req.Username]
	users.mu.RUnlock()
	if exists {
		json.NewEncoder(w).Encode(AdminResponse{
			Success: false,
			Message: "User already exists",
		})
		return
	}

	// Add the new user
	users.mu.Lock()
	users.Users[req.Username] = bucket.User{
		Password: req.Password,
		IsAdmin:  req.IsAdmin,
	}
	users.mu.Unlock()

	// Save to S3
	if err := saveUsers(); err != nil {
		log.Printf("Error saving users: %v", err)
		json.NewEncoder(w).Encode(AdminResponse{
			Success: false,
			Message: "Failed to save user",
		})
		return
	}

	json.NewEncoder(w).Encode(AdminResponse{
		Success: true,
		Message: "User added successfully",
	})
}

func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]

	// Check if user exists
	users.mu.RLock()
	_, exists := users.Users[username]
	users.mu.RUnlock()
	if !exists {
		json.NewEncoder(w).Encode(AdminResponse{
			Success: false,
			Message: "User not found",
		})
		return
	}

	// Delete the user
	users.mu.Lock()
	delete(users.Users, username)
	users.mu.Unlock()

	// Save to S3
	if err := saveUsers(); err != nil {
		log.Printf("Error saving users: %v", err)
		json.NewEncoder(w).Encode(AdminResponse{
			Success: false,
			Message: "Failed to delete user",
		})
		return
	}

	json.NewEncoder(w).Encode(AdminResponse{
		Success: true,
		Message: "User deleted successfully",
	})
}

func toggleAdminHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]

	var req ToggleAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Check if user exists and update admin status
	users.mu.Lock()
	user, exists := users.Users[username]
	if !exists {
		users.mu.Unlock()
		json.NewEncoder(w).Encode(AdminResponse{
			Success: false,
			Message: "User not found",
		})
		return
	}

	user.IsAdmin = req.IsAdmin
	users.Users[username] = user
	users.mu.Unlock()

	// Save to S3
	if err := saveUsers(); err != nil {
		log.Printf("Error saving users: %v", err)
		json.NewEncoder(w).Encode(AdminResponse{
			Success: false,
			Message: "Failed to update user",
		})
		return
	}

	json.NewEncoder(w).Encode(AdminResponse{
		Success: true,
		Message: "User updated successfully",
	})
}

// Post API handlers
func listPostsAPIHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	posts, err := listPosts()
	if err != nil {
		log.Printf("Error listing posts: %v", err)
		http.Error(w, "Failed to list posts", http.StatusInternalServerError)
		return
	}

	// Filter posts based on permissions
	var filteredPosts []PostListItem
	admin := isAdmin(username)
	for _, post := range posts {
		// Show all published posts, or drafts owned by user, or all if admin
		if post.Published || post.Author == username || admin {
			filteredPosts = append(filteredPosts, post)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filteredPosts)
}

func getPostAPIHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	post, err := loadPost(id)
	if err != nil {
		log.Printf("Error loading post %s: %v", id, err)
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	// Check if post is deleted
	if post.DeletedAt != nil {
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	// Check view permissions
	admin := isAdmin(username)
	if !canViewPost(post, username, admin) {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

func createPostAPIHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	if req.Author == "" {
		http.Error(w, "Author is required", http.StatusBadRequest)
		return
	}

	// Create new post
	now := time.Now().UTC()
	post := Post{
		ID:        generatePostID(req.Title),
		Title:     req.Title,
		Slug:      generateSlug(req.Title),
		Markdown:  req.Markdown,
		Author:    req.Author,
		CreatedBy: username,
		CreatedAt: now,
		UpdatedAt: now,
		Published: req.Published,
		Metadata: PostMetadata{
			Tags:      []string{},
			Category:  "",
			Excerpt:   generateExcerpt(req.Markdown),
			Recording: req.Recording,
		},
	}

	if req.Recording != "" {
		log.Printf("[POST_CREATE] Storing recording reference: '%s'", req.Recording)
	}

	if err := savePost(&post); err != nil {
		log.Printf("Error saving post: %v", err)
		http.Error(w, "Failed to save post", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

func updatePostAPIHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	// Load existing post
	post, err := loadPost(id)
	if err != nil {
		log.Printf("Error loading post %s: %v", id, err)
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	// Check if post is deleted
	if post.DeletedAt != nil {
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	// Check edit permissions
	admin := isAdmin(username)
	if !checkPostPermissions(post, username, admin) {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	var req UpdatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	if req.Author == "" {
		http.Error(w, "Author is required", http.StatusBadRequest)
		return
	}

	// Update post
	post.Title = req.Title
	post.Slug = generateSlug(req.Title)
	post.Markdown = req.Markdown
	post.Author = req.Author
	post.Published = req.Published
	post.UpdatedAt = time.Now().UTC()
	post.Metadata.Excerpt = generateExcerpt(req.Markdown)
	post.Metadata.Recording = req.Recording

	if err := savePost(post); err != nil {
		log.Printf("Error updating post: %v", err)
		http.Error(w, "Failed to update post", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

func deletePostAPIHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	// Load existing post
	post, err := loadPost(id)
	if err != nil {
		log.Printf("Error loading post %s: %v", id, err)
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	// Check if already deleted
	if post.DeletedAt != nil {
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	// Check delete permissions
	admin := isAdmin(username)
	if !checkPostPermissions(post, username, admin) {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	if err := deletePost(id); err != nil {
		log.Printf("Error deleting post: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Post deleted successfully",
	})
}

func uploadPostImageHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	postID := vars["id"]

	// Load post to verify ownership
	post, err := loadPost(postID)
	if err != nil {
		log.Printf("Error loading post %s: %v", postID, err)
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	// Check if post is deleted
	if post.DeletedAt != nil {
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	// Check edit permissions
	admin := isAdmin(username)
	if !checkPostPermissions(post, username, admin) {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Failed to get image from request", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file extension
	ext := filepath.Ext(header.Filename)
	allowedExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".webp": true,
	}
	if !allowedExts[strings.ToLower(ext)] {
		http.Error(w, "Invalid image format. Allowed: jpg, jpeg, png, gif, webp", http.StatusBadRequest)
		return
	}

	// Generate safe filename
	timestamp := time.Now().UTC().Format("20060102-150405")
	safeFilename := regexp.MustCompile(`[^a-zA-Z0-9._-]`).ReplaceAllString(header.Filename, "")
	filename := fmt.Sprintf("%s-%s", timestamp, safeFilename)

	// Store at posts/images/<post-id>/<filename>
	key := fmt.Sprintf("posts/images/%s/%s", postID, filename)

	// Upload with public-read ACL
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Create metadata for the image
	metadata := map[string]*string{
		"Post-Id":     aws.String(postID),
		"Uploaded-By": aws.String(username),
		"Upload-Time": aws.String(time.Now().UTC().Format(time.RFC3339)),
	}

	err = bucketClient.PutObjectWithMetadata(key, file, contentType, metadata, "public-read")
	if err != nil {
		log.Printf("Error uploading image: %v", err)
		http.Error(w, "Failed to upload image", http.StatusInternalServerError)
		return
	}

	// Return the public URL
	url := fmt.Sprintf("https://%s.nyc3.digitaloceanspaces.com/%s", bucketClient.Bucket, key)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url": url,
	})
}

func uploadBirthdayImageHandler(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form (10MB max)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Failed to get image from request", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file extension
	ext := filepath.Ext(header.Filename)
	allowedExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".webp": true,
	}
	if !allowedExts[strings.ToLower(ext)] {
		http.Error(w, "Invalid image format. Allowed: jpg, jpeg, png, gif, webp", http.StatusBadRequest)
		return
	}

	// Generate safe filename with timestamp
	timestamp := time.Now().UTC().Format("20060102-150405")
	safeFilename := regexp.MustCompile(`[^a-zA-Z0-9._-]`).ReplaceAllString(header.Filename, "")
	filename := fmt.Sprintf("%s-%s", timestamp, safeFilename)

	// Store at birthday/<filename>
	key := fmt.Sprintf("birthday/%s", filename)

	// Upload with public-read ACL
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Create metadata
	metadata := map[string]*string{
		"Upload-Time":       aws.String(time.Now().UTC().Format(time.RFC3339)),
		"Original-Filename": aws.String(header.Filename),
	}

	err = bucketClient.PutObjectWithMetadata(key, file, contentType, metadata, "public-read")
	if err != nil {
		log.Printf("Error uploading birthday image: %v", err)
		http.Error(w, "Failed to upload image", http.StatusInternalServerError)
		return
	}

	// Return the public URL
	url := fmt.Sprintf("https://%s.nyc3.digitaloceanspaces.com/%s", bucketClient.Bucket, key)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"url":     url,
		"message": "Image uploaded successfully",
	})
}

func listRecordingsAPIHandler(w http.ResponseWriter, r *http.Request) {
	// List all public recordings from the recordings/ directory
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	prefix := fmt.Sprintf("recordings/%s/", username)

	objects, err := bucketClient.ListObjects(prefix)
	if err != nil {
		log.Printf("Error listing recordings: %v", err)
		http.Error(w, "Failed to list recordings", http.StatusInternalServerError)
		return
	}

	type RecordingItem struct {
		Key          string `json:"key"`
		DisplayName  string `json:"displayName"`
		LastModified string `json:"lastModified"`
	}

	var recordings []RecordingItem
	for _, obj := range objects {
		if obj.Key == nil {
			continue
		}

		// Only include .mp3 files
		if len(*obj.Key) < 4 || (*obj.Key)[len(*obj.Key)-4:] != ".mp3" {
			continue
		}

		// Check if file is public by checking ACL
		aclOutput, err := bucketClient.GetObjectACL(*obj.Key)
		if err != nil {
			log.Printf("Error getting ACL for %s: %v", *obj.Key, err)
			continue
		}

		// Check if public-read
		isPublic := false
		for _, grant := range aclOutput.Grants {
			if grant.Grantee.URI != nil && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
				isPublic = true
				break
			}
		}

		if !isPublic {
			continue
		}

		// Get display name from metadata if available
		displayName := *obj.Key
		headOutput, err := bucketClient.HeadObject(*obj.Key)
		if err == nil && headOutput.Metadata != nil {
			if name, ok := headOutput.Metadata["Display-Name"]; ok && name != nil {
				displayName = *name
			}
		}

		lastModified := ""
		if obj.LastModified != nil {
			lastModified = obj.LastModified.Format(time.RFC3339)
		}

		recordings = append(recordings, RecordingItem{
			Key:          *obj.Key,
			DisplayName:  displayName,
			LastModified: lastModified,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recordings)
}

// Post page handlers
func postsListPageHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	posts, err := listPosts()
	if err != nil {
		log.Printf("Error listing posts: %v", err)
		http.Error(w, "Failed to list posts", http.StatusInternalServerError)
		return
	}

	// Filter posts based on permissions
	var filteredPosts []PostListItem
	admin := isAdmin(username)
	for _, post := range posts {
		// Show all published posts, or drafts owned by user, or all if admin
		if post.Published || post.Author == username || admin {
			filteredPosts = append(filteredPosts, post)
		}
	}

	data := struct {
		Posts    []PostListItem
		Username string
		IsAdmin  bool
	}{
		Posts:    filteredPosts,
		Username: username,
		IsAdmin:  admin,
	}

	if err := templates.ExecuteTemplate(w, "posts_list.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func postEditorPageHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	admin := isAdmin(username)
	var post *Post
	var err error
	var recordingKey string

	if id != "" {
		// Edit existing post
		post, err = loadPost(id)
		if err != nil {
			log.Printf("Error loading post %s: %v", id, err)
			http.Error(w, "Post not found", http.StatusNotFound)
			return
		}

		// Check if post is deleted
		if post.DeletedAt != nil {
			http.Error(w, "Post not found", http.StatusNotFound)
			return
		}

		// Check edit permissions
		if !checkPostPermissions(post, username, admin) {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}
	} else {
		// Check for recording parameter when creating new post
		recordingKey = r.URL.Query().Get("recording")
	}

	data := struct {
		Post         *Post
		Username     string
		IsAdmin      bool
		RecordingKey string
	}{
		Post:         post,
		Username:     username,
		IsAdmin:      admin,
		RecordingKey: recordingKey,
	}

	if err := templates.ExecuteTemplate(w, "post_editor.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func postViewPageHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, sessionName)
	username, ok := session.Values["username"].(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	post, err := loadPost(id)
	if err != nil {
		log.Printf("Error loading post %s: %v", id, err)
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	// Check if post is deleted
	if post.DeletedAt != nil {
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	// Check view permissions
	admin := isAdmin(username)
	if !canViewPost(post, username, admin) {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	canEdit := checkPostPermissions(post, username, admin)

	data := struct {
		Post     *Post
		Username string
		IsAdmin  bool
		CanEdit  bool
	}{
		Post:     post,
		Username: username,
		IsAdmin:  admin,
		CanEdit:  canEdit,
	}

	if err := templates.ExecuteTemplate(w, "post_view.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Helper functions
func isValidCredentials(username, password string) bool {
	users.mu.RLock()
	defer users.mu.RUnlock()

	user, exists := users.Users[username]
	if !exists {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(user.Password)) == 1
}

func isAdmin(username string) bool {
	users.mu.RLock()
	defer users.mu.RUnlock()

	user, exists := users.Users[username]
	return exists && user.IsAdmin
}

func loadUsers() error {
	result, err := bucketClient.GetObject(userFile)
	if err != nil {
		log.Printf("Error getting users file (might not exist yet): %v", err)
		// Initialize with empty store if file doesn't exist
		users.mu.Lock()
		users.Users = make(map[string]bucket.User)
		users.mu.Unlock()
		return nil
	}
	defer result.Body.Close()

	data, err := ioutil.ReadAll(result.Body)
	if err != nil {
		return fmt.Errorf("error reading users file: %v", err)
	}

	var store bucket.UserStore
	if err := json.Unmarshal(data, &store); err != nil {
		return fmt.Errorf("error parsing users file: %v", err)
	}

	// Only lock when we're ready to update
	users.mu.Lock()
	users.Users = store.Users
	users.mu.Unlock()

	return nil
}

func saveUsers() error {
	users.mu.RLock()
	store := bucket.UserStore{
		Users: users.Users,
	}
	users.mu.RUnlock()

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("error encoding users: %v", err)
	}

	err = bucketClient.PutObject(userFile, data, "application/json")
	if err != nil {
		return fmt.Errorf("error uploading users file: %v", err)
	}

	return nil
}

// Middleware
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[AUTH] Request to %s", r.URL.Path)
		session, err := store.Get(r, sessionName)
		if err != nil {
			log.Printf("[AUTH] Session error: %v", err)
			http.Error(w, "Session error", http.StatusInternalServerError)
			return
		}

		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			log.Printf("[AUTH] Not authenticated, redirecting to /")
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		log.Printf("[AUTH] Authenticated, proceeding to handler")
		next.ServeHTTP(w, r)
	})
}

func adminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := store.Get(r, sessionName)
		if err != nil {
			http.Error(w, "Session error", http.StatusInternalServerError)
			return
		}

		username, ok := session.Values["username"].(string)
		if !ok || !isAdmin(username) {
			http.Error(w, "Unauthorized - Admin access required", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[HOME] Request to %s", r.URL.Path)
	// Only handle the root path
	if r.URL.Path != "/" {
		log.Printf("[HOME] Not root path, returning 404")
		http.NotFound(w, r)
		return
	}

	session, err := store.Get(r, sessionName)
	if err != nil {
		log.Printf("[HOME] Session error: %v", err)
		session = sessions.NewSession(store, sessionName)
	}

	if auth, ok := session.Values["authenticated"].(bool); ok && auth {
		log.Printf("[HOME] User authenticated, redirecting to /files")
		// Redirect to files page
		http.Redirect(w, r, "/files", http.StatusSeeOther)
		return
	}

	log.Printf("[HOME] Showing login page")
	if err := templates.ExecuteTemplate(w, "login.html", nil); err != nil {
		log.Printf("[HOME] Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[HOME] Template execution complete")
}

func rateLimitMiddleware(next http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Every(1*time.Second), 3)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' cdn.jsdelivr.net maxcdn.bootstrapcdn.com; font-src 'self' maxcdn.bootstrapcdn.com; img-src 'self' data: https://cabbagetown.nyc3.digitaloceanspaces.com;")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		fmt.Println("CORS request", r.Header.Get("Origin"))
		// Allow requests from localhost:8000
		origin := r.Header.Get("Origin")
		if origin == "http://localhost:8000" || origin == "http://127.0.0.1:8000" || origin == "http://[::1]:8000" || origin == "https://birthday.cabbage.town" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		// Handle preflight OPTIONS request
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func setupRoutes() *mux.Router {
	r := mux.NewRouter().StrictSlash(true)

	log.Printf("[SETUP] Configuring routes")

	// Add global middleware
	r.Use(corsMiddleware)
	r.Use(securityHeadersMiddleware)

	// Static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Public routes - must come before protected routes
	r.HandleFunc("/", homeHandler).Methods("GET").Name("home")
	r.HandleFunc("/login", loginHandler).Methods("POST").Name("login")
	r.HandleFunc("/logout", logoutHandler).Methods("GET").Name("logout")
	r.HandleFunc("/api/birthday", uploadBirthdayImageHandler).Methods("POST").Name("birthday-upload")

	// Protected routes with rate limiting
	protected := r.NewRoute().Subrouter()
	protected.Use(authMiddleware)
	protected.Use(rateLimitMiddleware)

	// File endpoints
	protected.HandleFunc("/files", listFilesHandler).Methods("GET")
	protected.HandleFunc("/upload", uploadPageHandler).Methods("GET")
	protected.HandleFunc("/api/upload", uploadHandler).Methods("POST")
	protected.HandleFunc("/api/files/toggle-access", toggleAccessHandler).Methods("POST")
	protected.HandleFunc("/api/files/rename", renameFileHandler).Methods("POST")
	protected.HandleFunc("/files/{key:.+}", viewFileHandler).Methods("GET")

	// Post page endpoints
	protected.HandleFunc("/posts", postsListPageHandler).Methods("GET")
	protected.HandleFunc("/posts/new", func(w http.ResponseWriter, r *http.Request) {
		postEditorPageHandler(w, r)
	}).Methods("GET")
	protected.HandleFunc("/posts/{id}", postViewPageHandler).Methods("GET")
	protected.HandleFunc("/posts/{id}/edit", postEditorPageHandler).Methods("GET")

	// Post API endpoints
	protected.HandleFunc("/api/posts", listPostsAPIHandler).Methods("GET")
	protected.HandleFunc("/api/posts", createPostAPIHandler).Methods("POST")
	protected.HandleFunc("/api/posts/{id}", getPostAPIHandler).Methods("GET")
	protected.HandleFunc("/api/posts/{id}", updatePostAPIHandler).Methods("PUT")
	protected.HandleFunc("/api/posts/{id}", deletePostAPIHandler).Methods("DELETE")
	protected.HandleFunc("/api/posts/{id}/images", uploadPostImageHandler).Methods("POST")
	protected.HandleFunc("/api/recordings", listRecordingsAPIHandler).Methods("GET")

	// Admin endpoints
	admin := protected.PathPrefix("/api/admin").Subrouter()
	admin.Use(adminMiddleware)
	admin.HandleFunc("/users", addUserHandler).Methods("POST")
	admin.HandleFunc("/users/{username}", deleteUserHandler).Methods("DELETE")
	admin.HandleFunc("/users/{username}/toggle-admin", toggleAdminHandler).Methods("POST")

	// Admin pages
	protected.HandleFunc("/admin/users", adminUsersPageHandler).Methods("GET")

	// Log all registered routes
	r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, err := route.GetPathTemplate()
		if err == nil {
			methods, _ := route.GetMethods()
			log.Printf("[SETUP] Route: %v %v", methods, pathTemplate)
		}
		return nil
	})

	return r
}

func init() {
	// Load .env file from parent directory
	if err := godotenv.Load("../.env"); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// Initialize session store
	sessionKey := os.Getenv("SESSION_KEY")
	if sessionKey == "" {
		log.Fatal("SESSION_KEY environment variable is required")
	}
	store = sessions.NewCookieStore([]byte(sessionKey))

	// Configure session security
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}

	// Initialize bucket client
	var err error
	bucketClient, err = bucket.NewClient()
	if err != nil {
		log.Fatalf("Failed to create bucket client: %v", err)
	}

	// Load users from S3
	if err := loadUsers(); err != nil {
		log.Printf("Warning: Could not load users from S3: %v", err)
	}
}

func startUserRefresh(ctx context.Context) {
	userRefreshTicker = time.NewTicker(5 * time.Minute)
	userRefreshDone = make(chan bool)

	go func() {
		for {
			select {
			case <-userRefreshTicker.C:
				if err := loadUsers(); err != nil {
					log.Printf("[USER REFRESH] Error refreshing users: %v", err)
				} else {
					log.Printf("[USER REFRESH] Successfully refreshed users list")
				}
			case <-userRefreshDone:
				userRefreshTicker.Stop()
				return
			case <-ctx.Done():
				userRefreshTicker.Stop()
				return
			}
		}
	}()
}

func main() {
	router := setupRoutes()

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the user refresh goroutine
	startUserRefresh(ctx)
	defer func() {
		if userRefreshDone != nil {
			userRefreshDone <- true
		}
	}()

	s := &http.Server{
		Addr:           ":8080",
		Handler:        router,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := s.Shutdown(shutdownCtx); err != nil {
			log.Printf("Error during server shutdown: %v", err)
		}
	}()

	log.Printf("Server starting on port 8080...")
	if err := s.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// Add this function near other helper functions
func sanitizeAndValidateKey(key string) error {
	// Prevent directory traversal
	if strings.Contains(key, "..") || strings.Contains(key, "./") {
		return fmt.Errorf("invalid path characters")
	}

	// Ensure path starts with recordings/
	if !strings.HasPrefix(key, "recordings/") {
		return fmt.Errorf("invalid path prefix")
	}

	// Validate path structure (recordings/<username>/<filename>)
	parts := strings.Split(key, "/")
	if len(parts) != 3 {
		return fmt.Errorf("invalid path structure")
	}

	// Validate username part contains no special characters
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(parts[1]) {
		return fmt.Errorf("invalid username in path")
	}

	// Validate filename
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+\.[a-zA-Z0-9]+$`).MatchString(parts[2]) {
		return fmt.Errorf("invalid filename")
	}

	return nil
}

// Post helper functions
func generateSlug(title string) string {
	// Convert to lowercase
	slug := strings.ToLower(title)
	// Replace spaces with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")
	// Remove all non-alphanumeric characters except hyphens
	slug = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(slug, "")
	// Remove multiple consecutive hyphens
	slug = regexp.MustCompile(`-+`).ReplaceAllString(slug, "-")
	// Trim hyphens from start and end
	slug = strings.Trim(slug, "-")
	// Limit length to 100 characters
	if len(slug) > 100 {
		slug = slug[:100]
	}
	return slug
}

func generateExcerpt(markdown string) string {
	// Strip markdown formatting
	text := markdown

	// Remove headers (# ## ###)
	text = regexp.MustCompile(`(?m)^#+\s+`).ReplaceAllString(text, "")

	// Remove links [text](url) -> text
	text = regexp.MustCompile(`\[([^\]]+)\]\([^\)]+\)`).ReplaceAllString(text, "$1")

	// Remove images ![alt](url)
	text = regexp.MustCompile(`!\[([^\]]*)\]\([^\)]+\)`).ReplaceAllString(text, "")

	// Remove bold/italic (**text** or *text*)
	text = regexp.MustCompile(`\*+([^*]+)\*+`).ReplaceAllString(text, "$1")

	// Remove code blocks ```code```
	text = regexp.MustCompile("(?s)```[^`]+```").ReplaceAllString(text, "")

	// Remove inline code `code`
	text = regexp.MustCompile("`([^`]+)`").ReplaceAllString(text, "$1")

	// Remove extra whitespace
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	// Truncate to ~200 chars at word boundary
	maxLen := 200
	if len(text) <= maxLen {
		return text
	}

	// Find last space before maxLen
	truncated := text[:maxLen]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > 0 {
		truncated = text[:lastSpace]
	}

	return truncated + "..."
}

func generatePostID(title string) string {
	timestamp := time.Now().UTC().Format("2006-01-02-150405")
	slug := generateSlug(title)
	return fmt.Sprintf("%s-%s", timestamp, slug)
}

func getPostKey(id string) string {
	return fmt.Sprintf("posts/%s.json", id)
}

func loadPost(id string) (*Post, error) {
	key := getPostKey(id)
	result, err := bucketClient.GetObject(key)
	if err != nil {
		return nil, fmt.Errorf("error getting post: %v", err)
	}
	defer result.Body.Close()

	data, err := ioutil.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading post: %v", err)
	}

	var post Post
	if err := json.Unmarshal(data, &post); err != nil {
		return nil, fmt.Errorf("error parsing post: %v", err)
	}

	return &post, nil
}

func savePost(post *Post) error {
	data, err := json.MarshalIndent(post, "", "  ")
	if err != nil {
		return fmt.Errorf("error encoding post: %v", err)
	}

	key := getPostKey(post.ID)
	err = bucketClient.PutObject(key, data, "application/json")
	if err != nil {
		return fmt.Errorf("error uploading post: %v", err)
	}

	return nil
}

func listPosts() ([]PostListItem, error) {
	objects, err := bucketClient.ListObjects("posts/")
	if err != nil {
		return nil, fmt.Errorf("failed to list posts: %v", err)
	}

	var posts []PostListItem
	for _, obj := range objects {
		// Skip directories and non-JSON files
		if strings.HasSuffix(*obj.Key, "/") || !strings.HasSuffix(*obj.Key, ".json") {
			continue
		}

		// Load the full post to get its data
		id := strings.TrimPrefix(*obj.Key, "posts/")
		id = strings.TrimSuffix(id, ".json")

		post, err := loadPost(id)
		if err != nil {
			log.Printf("Error loading post %s: %v", id, err)
			continue
		}

		// Skip soft-deleted posts
		if post.DeletedAt != nil {
			continue
		}

		// Generate excerpt from markdown (first 200 chars)
		excerpt := post.Markdown
		if len(excerpt) > 200 {
			excerpt = excerpt[:200] + "..."
		}
		// Remove newlines from excerpt
		excerpt = strings.ReplaceAll(excerpt, "\n", " ")

		posts = append(posts, PostListItem{
			ID:        post.ID,
			Title:     post.Title,
			Slug:      post.Slug,
			Author:    post.Author,
			CreatedAt: post.CreatedAt,
			UpdatedAt: post.UpdatedAt,
			Published: post.Published,
			Excerpt:   excerpt,
		})
	}

	// Sort by created date, newest first
	for i := 0; i < len(posts); i++ {
		for j := i + 1; j < len(posts); j++ {
			if posts[i].CreatedAt.Before(posts[j].CreatedAt) {
				posts[i], posts[j] = posts[j], posts[i]
			}
		}
	}

	return posts, nil
}

func deletePost(id string) error {
	// Load the post
	post, err := loadPost(id)
	if err != nil {
		return fmt.Errorf("post not found: %v", err)
	}

	// Soft delete: set DeletedAt timestamp
	now := time.Now().UTC()
	post.DeletedAt = &now

	// Save the post with the DeletedAt field
	if err := savePost(post); err != nil {
		return fmt.Errorf("failed to soft delete post: %v", err)
	}

	return nil
}

func checkPostPermissions(post *Post, username string, isAdmin bool) bool {
	if isAdmin {
		return true
	}
	return post.CreatedBy == username
}

func canViewPost(post *Post, username string, isAdmin bool) bool {
	if post.Published {
		return true // Anyone authenticated can view published posts
	}
	return post.Author == username || isAdmin // Only author/admin can view drafts
}
