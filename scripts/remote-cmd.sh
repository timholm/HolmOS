#!/bin/bash
# remote-cmd.sh - Run commands on Mac Mini via HTTP API
# Usage: ./remote-cmd.sh "command to run"
#
# Environment variables:
#   REMOTE_API_URL - The ngrok URL for the command API (required)
#
# Example:
#   export REMOTE_API_URL="https://xxxxx.ngrok-free.app"
#   ./remote-cmd.sh "kubectl get pods -n holm"

set -e

# Check for required environment variable
if [ -z "$REMOTE_API_URL" ]; then
    echo "Error: REMOTE_API_URL environment variable not set"
    echo "Set it to your ngrok HTTP API URL, e.g.:"
    echo "  export REMOTE_API_URL=\"https://xxxxx.ngrok-free.app\""
    exit 1
fi

# Check for command argument
if [ -z "$1" ]; then
    echo "Usage: $0 \"command to run\""
    echo "Example: $0 \"hostname && uname -a\""
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
