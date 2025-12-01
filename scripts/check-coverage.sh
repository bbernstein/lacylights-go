#!/bin/bash
# scripts/check-coverage.sh
# Checks test coverage against per-package thresholds
# Excludes auto-generated code from coverage reporting

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Coverage thresholds by package (percentage)
# Packages with 100% should maintain 100%
# Other testable packages start at current coverage as floor
declare -A THRESHOLDS=(
    ["github.com/bbernstein/lacylights-go/pkg/artnet"]=100
    ["github.com/bbernstein/lacylights-go/internal/config"]=100
    ["github.com/bbernstein/lacylights-go/internal/services/pubsub"]=100
    ["github.com/bbernstein/lacylights-go/internal/services/fade"]=85
    ["github.com/bbernstein/lacylights-go/internal/services/dmx"]=80
    ["github.com/bbernstein/lacylights-go/internal/services/network"]=60
    ["github.com/bbernstein/lacylights-go/internal/services/preview"]=35
    ["github.com/bbernstein/lacylights-go/internal/services/playback"]=15
    ["github.com/bbernstein/lacylights-go/internal/services/export"]=8
    ["github.com/bbernstein/lacylights-go/internal/graphql/resolvers"]=15
)

# Packages to skip coverage checks (auto-generated, main, etc.)
SKIP_PACKAGES=(
    "github.com/bbernstein/lacylights-go/internal/graphql/generated"
    "github.com/bbernstein/lacylights-go/cmd/server"
    "github.com/bbernstein/lacylights-go/internal/database"
    "github.com/bbernstein/lacylights-go/internal/database/models"
    "github.com/bbernstein/lacylights-go/internal/database/repositories"
    "github.com/bbernstein/lacylights-go/internal/services/import"
)

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
    # Also match lines without "ok" prefix for packages with 0% coverage
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
        skip=false
        for skip_pkg in "${SKIP_PACKAGES[@]}"; do
            if [[ "$pkg" == "$skip_pkg" ]]; then
                skip=true
                break
            fi
        done

        if [[ "$skip" == "true" ]]; then
            echo -e "${YELLOW}SKIP${NC} $pkg (excluded from coverage checks)"
            continue
        fi

        # Get threshold for this package
        threshold="${THRESHOLDS[$pkg]}"

        if [[ -z "$threshold" ]]; then
            echo -e "${YELLOW}WARN${NC} $pkg: ${coverage}% (no threshold defined)"
            continue
        fi

        # Compare coverage to threshold (using bc for floating point)
        if echo "$coverage >= $threshold" | bc -l | grep -q 1; then
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
