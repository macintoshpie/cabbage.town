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

### Generate playlists and RSS feed
```bash
go run ./cmd/trellis
```

### Update ACLs for recent recordings
```bash
go run ./cmd/update_acls
```

### Add ID3 metadata to recent recordings
```bash
go run ./cmd/add_metadata
```

### Test metadata processing (dry run)
```bash
go run ./cmd/add_metadata -dry-run
```
This downloads files, applies metadata, and saves them locally without uploading back to the bucket.

## Automated Workflow

The GitHub Actions workflow runs daily at midnight ET and:
1. Updates ACLs for recordings modified in the last 72 hours
2. Adds ID3 metadata (title, artist, album, year, genre) to recent recordings that haven't been processed yet
3. Regenerates playlists and RSS feed
4. Commits changes to the repository

## Metadata Processing Notes

- Files are marked with `id3-processed=true` metadata to prevent reprocessing
- Existing object metadata and ACL permissions are preserved when updating files
- Only files modified in the last 72 hours that lack the processed flag are updated