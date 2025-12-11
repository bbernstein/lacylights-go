#!/bin/bash
# Download the Open Fixture Library zipball for embedding in the binary
# This script downloads the latest OFL data from GitHub and saves it to the data directory

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
DATA_DIR="$PROJECT_ROOT/internal/services/ofl/data"
OUTPUT_FILE="$DATA_DIR/ofl-bundle.zip"

# GitHub API URL for the zipball
OFL_REPO="OpenLightingProject/open-fixture-library"
OFL_API_URL="https://api.github.com/repos/$OFL_REPO/zipball/master"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Downloading Open Fixture Library...${NC}"
echo "Target: $OUTPUT_FILE"

# Create data directory if it doesn't exist
mkdir -p "$DATA_DIR"

# Download the zipball
echo -e "${YELLOW}Fetching from GitHub...${NC}"
HTTP_CODE=$(curl -L -w "%{http_code}" -o "$OUTPUT_FILE.tmp" "$OFL_API_URL" 2>/dev/null)

if [ "$HTTP_CODE" -eq 200 ]; then
    mv "$OUTPUT_FILE.tmp" "$OUTPUT_FILE"
    FILE_SIZE=$(du -h "$OUTPUT_FILE" | cut -f1)
    echo -e "${GREEN}Successfully downloaded OFL bundle ($FILE_SIZE)${NC}"

    # Show some stats about the download
    echo -e "${YELLOW}Bundle statistics:${NC}"
    unzip -l "$OUTPUT_FILE" 2>/dev/null | tail -1 || echo "  (zip listing not available)"

    # Extract and count fixtures
    FIXTURE_COUNT=$(unzip -l "$OUTPUT_FILE" 2>/dev/null | grep -c '\.json$' || echo "unknown")
    echo "  JSON files: $FIXTURE_COUNT"
else
    rm -f "$OUTPUT_FILE.tmp"
    echo -e "${RED}Failed to download OFL bundle (HTTP $HTTP_CODE)${NC}"
    echo "Make sure you have internet access and GitHub is reachable."
    exit 1
fi

echo -e "${GREEN}Done!${NC}"
