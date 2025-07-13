# Trellis

Automated tools for managing Cabbage Town Radio recordings, playlists, and metadata.

## Setup

### Prerequisites

1. **Go** (version 1.21 or later)
   ```bash
   # Install Go from https://golang.org/dl/
   # Or on macOS with Homebrew:
   brew install go
   ```

2. **Python** and **eyeD3** (for ID3 metadata processing)
   ```bash
   # Install Python from https://python.org
   # Or on macOS with Homebrew:
   brew install python
   
   # Install eyeD3
   pip install eyeD3
   ```

3. **Environment Variables**
   ```bash
   export DO_ACCESS_KEY_ID="your_digitalocean_access_key"
   export DO_SECRET_ACCESS_KEY="your_digitalocean_secret_key"
   ```

### Local Development

1. Clone the repository and navigate to the trellis directory
2. Install dependencies:
   ```bash
   go mod tidy
   ```

## Usage

### Full recordings update workflow (recommended)
```bash
# Run complete workflow
go run ./cmd/update_recordings

# Dry run to see what would be changed
go run ./cmd/update_recordings -dry-run

# Skip specific steps
go run ./cmd/update_recordings -skip-acl
go run ./cmd/update_recordings -skip-metadata  
go run ./cmd/update_recordings -skip-playlists
```

### Individual commands (for development/debugging)

#### Generate playlists and RSS feed only
```bash
go run ./cmd/trellis
```

#### Update ACLs for recent recordings only
```bash
go run ./cmd/update_acls
```

#### Add ID3 metadata to recent recordings only
```bash
go run ./cmd/add_metadata

# Test metadata processing (dry run)
go run ./cmd/add_metadata -dry-run
```

## Automated Workflow

The GitHub Actions workflow runs daily at midnight ET using the unified `update_recordings` command:
1. **Update ACLs** - Makes recent recordings public (respects manual privacy settings)
2. **Add ID3 metadata** - Adds title, artist, album, year, genre to unprocessed files
3. **Generate playlists/feed** - Updates M3U playlists and RSS feed
4. **Commit changes** - Automatically commits updated playlists/feed to git

You can run the same workflow locally:
```bash
go run ./cmd/update_recordings -dry-run  # Test first
go run ./cmd/update_recordings           # Run for real
```

## Metadata Processing Notes

- Files are marked with `id3-processed=true` metadata to prevent reprocessing
- Existing object metadata and ACL permissions are preserved when updating files
- Only files modified in the last 72 hours that lack the processed flag are updated