#!/bin/bash

# Build script for Trellis executable
set -e

echo "🔨 Building Trellis executable..."

# Create bin directory if it doesn't exist
mkdir -p bin

# Build update_recordings command
echo "Building update_recordings..."
go build -o bin/update_recordings ./cmd/update_recordings

echo "✅ Executable built successfully in bin/ directory"
echo ""
echo "Usage:"
echo "  bin/update_recordings [OPTIONS] [SUBCOMMAND]"
echo ""
echo "Subcommands:"
echo "  all        Run all steps (default)"
echo "  acls       Update ACLs for recent recordings only"
echo "  metadata   Add ID3 metadata to recent recordings only"
echo "  playlists  Generate playlists and RSS feed only"
echo ""
echo "Options:"
echo "  -dry-run        Perform a dry run without making changes"
echo "  -skip-acl       Skip ACL updates"
echo "  -skip-metadata  Skip ID3 metadata processing"
echo "  -skip-playlists Skip playlist/feed generation"