package main

import (
	"log"

	"cabbage.town/trellis/internal/acls"
)

func main() {
	log.Println("Starting ACL update process...")
	
	err := acls.UpdateACLs(false) // not a dry run
	if err != nil {
		log.Fatalf("Error updating ACLs: %v", err)
	}
	
	log.Println("ACL update process complete!")
}
