#!/bin/bash

BUCKET_URL="https://cabbagetown.nyc3.digitaloceanspaces.com"
LIST_URL="$BUCKET_URL/?prefix=recordings/&max-keys=1000"
OUTPUT_FILE="public/playlists/recordings.m3u"

echo "Creating directory for output file..."
mkdir -p "$(dirname "$OUTPUT_FILE")"

echo "Initializing playlist file..."
echo "#EXTM3U" > "$OUTPUT_FILE"

echo "Fetching list of recordings from $LIST_URL..."
response=$(curl -s "$LIST_URL")
echo "Response from LIST_URL: $response"  # Log the response to the console
echo "$response" |
  grep '<Key>' |
  sed -E 's|.*<Key>(.*\.mp3)</Key>.*|\1|' |
  sort |
  while read -r key; do
    full_url="$BUCKET_URL/$key"
    echo "Checking URL: $full_url"
    headers=$(curl -s -I "$full_url")
    echo "Headers: $headers"
    if echo "$headers" | grep -q "HTTP/.* 200"; then
      echo "Adding $full_url to playlist..."
      echo "#EXTINF:-1,$(basename "$key")" >> "$OUTPUT_FILE"
      echo "$full_url" >> "$OUTPUT_FILE"
    else
      echo "Failed to access $full_url"
    fi
  done

echo "Playlist update complete."