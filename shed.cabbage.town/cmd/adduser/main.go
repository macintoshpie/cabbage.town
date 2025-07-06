package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/joho/godotenv"

	"cabbage.town/shed.cabbage.town/internal/bucket"
)

const userFile = "shed/users.json"

func main() {
	// Parse command line flags
	username := flag.String("user", "", "Username to add")
	password := flag.String("pass", "", "Password for the user")
	isAdmin := flag.Bool("admin", false, "Whether the user should have admin privileges")
	flag.Parse()

	if *username == "" || *password == "" {
		fmt.Println("Usage: adduser -user <username> -pass <password> [-admin]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Load environment variables
	if err := godotenv.Load("../../../.env"); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// Initialize bucket client
	bucketClient, err := bucket.NewClient()
	if err != nil {
		log.Fatalf("Failed to create bucket client: %v", err)
	}

	// Try to load existing users file
	var store bucket.UserStore
	result, err := bucketClient.GetObject(userFile)
	if err != nil {
		// Initialize new store if file doesn't exist
		store = bucket.UserStore{
			Users: make(map[string]bucket.User),
		}
	} else {
		// Read and parse existing file
		data, err := ioutil.ReadAll(result.Body)
		if err != nil {
			log.Fatalf("Error reading users file: %v", err)
		}
		if err := json.Unmarshal(data, &store); err != nil {
			log.Fatalf("Error parsing users file: %v", err)
		}
	}

	// Check if user already exists
	if _, exists := store.Users[*username]; exists {
		log.Fatalf("User %s already exists", *username)
	}

	// Add new user
	store.Users[*username] = bucket.User{
		Password: *password,
		IsAdmin:  *isAdmin,
	}

	// Convert updated store to JSON
	updatedData, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		log.Fatalf("Error encoding users data: %v", err)
	}

	// Upload updated file to S3
	err = bucketClient.PutObject(userFile, updatedData, "application/json")
	if err != nil {
		log.Fatalf("Error uploading users file: %v", err)
	}

	fmt.Printf("Successfully added user: %s (admin: %v)\n", *username, *isAdmin)
}
