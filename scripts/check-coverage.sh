#!/bin/bash
# scripts/check-coverage.sh
# Checks test coverage against per-package thresholds
# Excludes auto-generated code from coverage reporting

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get threshold for a package
# These thresholds represent high-water marks - coverage should not drop below these values
# Update thresholds when coverage improves to prevent regression
get_threshold() {
    local pkg="$1"
    case "$pkg" in
        "pkg/artnet") echo 100 ;;
        "internal/config") echo 100 ;;
        "internal/services/pubsub") echo 100 ;;
        "internal/services/preview") echo 91 ;;  # Integration tests provide good coverage
        "internal/services/fade") echo 91 ;;
        "internal/services/network") echo 64 ;;  # macOS-specific code paths not covered on Linux CI
        "internal/services/export") echo 87 ;;  # Integration tests provide good coverage
        "internal/services/dmx") echo 85 ;;
        "internal/services/playback") echo 82 ;;  # Integration tests provide good coverage
        "internal/services/import") echo 78 ;;  # Integration tests provide good coverage
        "internal/graphql/resolvers") echo 17 ;;  # Auto-generated code, coverage via service tests
        *) echo "" ;;
    esac
}

# Check if package should be skipped
should_skip() {
    local pkg="$1"
    case "$pkg" in
        "github.com/bbernstein/lacylights-go/internal/graphql/generated") return 0 ;;
        "github.com/bbernstein/lacylights-go/cmd/server") return 0 ;;
        "github.com/bbernstein/lacylights-go/internal/database") return 0 ;;
        "github.com/bbernstein/lacylights-go/internal/database/models") return 0 ;;
        "github.com/bbernstein/lacylights-go/internal/database/repositories") return 0 ;;
        "github.com/bbernstein/lacylights-go/internal/services/testutil") return 0 ;;  # Test utilities only
        *) return 1 ;;
    esac
}

echo "Running tests with coverage..."
echo ""

# Run tests and capture output
TEST_OUTPUT=$(ARTNET_ENABLED=false go test -cover ./... 2>&1)
TEST_EXIT_CODE=$?

# Show test output (filtered)
echo "$TEST_OUTPUT" | grep -E "^(ok|FAIL|\?)" || true

if [ $TEST_EXIT_CODE -ne 0 ]; then
    echo ""
    echo -e "${RED}Tests failed!${NC}"
    exit 1
fi

echo ""
echo "Checking coverage thresholds..."
echo ""

# Parse coverage output
FAILED=0
PASSED=0

# Process each line from test output
while IFS= read -r line; do
    # Match lines like: ok  	github.com/bbernstein/lacylights-go/internal/config	0.016s	coverage: 100.0% of statements
    if [[ $line =~ (github\.com/bbernstein/lacylights-go/[^[:space:]]+) ]]; then
        pkg="${BASH_REMATCH[1]}"

        # Check if this line has coverage info
        if [[ $line =~ coverage:\ ([0-9.]+)%\ of\ statements ]]; then
            coverage="${BASH_REMATCH[1]}"
        elif [[ $line =~ coverage:\ 0\.0%\ of\ statements ]]; then
            coverage="0.0"
        else
            # No coverage info on this line, skip
            continue
        fi

        # Check if package should be skipped
        if should_skip "$pkg"; then
            echo -e "${YELLOW}SKIP${NC} $pkg (excluded from coverage checks)"
            continue
        fi

        # Get threshold for this package (strip the prefix for lookup)
        short_pkg="${pkg#github.com/bbernstein/lacylights-go/}"
        threshold=$(get_threshold "$short_pkg")

        if [[ -z "$threshold" ]]; then
            echo -e "${YELLOW}WARN${NC} $pkg: ${coverage}% (no threshold defined)"
            continue
        fi

        # Compare coverage to threshold (using awk for floating point)
        if awk "BEGIN {exit !($coverage >= $threshold)}"; then
            echo -e "${GREEN}PASS${NC} $pkg: ${coverage}% >= ${threshold}%"
            PASSED=$((PASSED + 1))
        else
            echo -e "${RED}FAIL${NC} $pkg: ${coverage}% < ${threshold}%"
            FAILED=$((FAILED + 1))
        fi
    fi
done <<< "$TEST_OUTPUT"

echo ""
echo "=================================="
echo "Coverage Summary"
echo "=================================="
echo -e "Passed: ${GREEN}${PASSED}${NC}"
echo -e "Failed: ${RED}${FAILED}${NC}"

if [ $FAILED -gt 0 ]; then
    echo ""
    echo -e "${RED}Coverage check failed!${NC}"
    echo "Some packages are below their coverage thresholds."
    echo "Please add tests to increase coverage."
    exit 1
else
    echo ""
    echo -e "${GREEN}All coverage thresholds met!${NC}"
    exit 0
fi
