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
	mergedMetadata["manually-privated"] = aws.String(fmt.Sprintf("%v", !req.MakePublic))
	mergedMetadata["privacy-timestamp"] = aws.String(time.Now().UTC().Format(time.RFC3339))

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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline';")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func setupRoutes() *mux.Router {
	r := mux.NewRouter().StrictSlash(true)

	log.Printf("[SETUP] Configuring routes")

	// Add global middleware
	r.Use(securityHeadersMiddleware)

	// Static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Public routes - must come before protected routes
	r.HandleFunc("/", homeHandler).Methods("GET").Name("home")
	r.HandleFunc("/login", loginHandler).Methods("POST").Name("login")
	r.HandleFunc("/logout", logoutHandler).Methods("GET").Name("logout")

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
