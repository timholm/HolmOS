#!/bin/bash
# remote-cmd.sh - Run commands on Mac Mini via HTTP API
# Usage: ./remote-cmd.sh "command to run"
#
# Default endpoint: https://cmd.holm.chat
# Override with: REMOTE_API_URL=https://other.url ./remote-cmd.sh "cmd"

set -e

# Default to cmd.holm.chat
REMOTE_API_URL="${REMOTE_API_URL:-https://cmd.holm.chat}"

# Check for command argument
if [ -z "$1" ]; then
    echo "Usage: $0 \"command to run\""
    echo "Example: $0 \"hostname && uname -a\""
    echo ""
    echo "Endpoint: $REMOTE_API_URL"
    exit 1
fi

CMD="$1"

# Run the command via HTTP API
RESPONSE=$(curl -s -X POST "${REMOTE_API_URL}/run" \
    -H "Content-Type: application/json" \
    -d "{\"cmd\": \"$CMD\"}")

# Parse response
STDOUT=$(echo "$RESPONSE" | jq -r '.stdout // empty')
STDERR=$(echo "$RESPONSE" | jq -r '.stderr // empty')
CODE=$(echo "$RESPONSE" | jq -r '.code // "1"')
ERROR=$(echo "$RESPONSE" | jq -r '.error // empty')

# Output results
if [ -n "$ERROR" ]; then
    echo "API Error: $ERROR" >&2
    exit 1
fi

if [ -n "$STDOUT" ]; then
    echo "$STDOUT"
fi

if [ -n "$STDERR" ]; then
    echo "$STDERR" >&2
fi

exit "$CODE"
