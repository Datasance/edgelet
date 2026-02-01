#!/bin/bash
# Performance profiling script for ioFog Agent

set -e

BINARY=${1:-"./build/iofog-agentd"}
PROFILE_OUTPUT=${2:-"build/profile.out"}
PROFILE_DURATION=${3:-30}

if [ ! -f "$BINARY" ]; then
    echo "Error: Binary not found at $BINARY"
    echo "Usage: $0 [binary] [output] [duration_seconds]"
    exit 1
fi

echo "Starting performance profiling..."
echo "Binary: $BINARY"
echo "Output: $PROFILE_OUTPUT"
echo "Duration: ${PROFILE_DURATION}s"
echo ""

# Start the binary with CPU profiling enabled
echo "Starting agent with CPU profiling..."
$BINARY &
AGENT_PID=$!

# Wait a bit for startup
sleep 2

# Start CPU profiling
echo "Collecting CPU profile for ${PROFILE_DURATION} seconds..."
go tool pprof -proto -output="$PROFILE_OUTPUT" http://localhost:54321/debug/pprof/profile?seconds=$PROFILE_DURATION 2>/dev/null || {
    echo "Warning: Could not collect profile via HTTP endpoint"
    echo "Falling back to signal-based profiling..."
    
    # Send SIGUSR1 to trigger profiling (if supported)
    kill -USR1 $AGENT_PID 2>/dev/null || true
    
    # Wait for profile duration
    sleep $PROFILE_DURATION
    
    # Stop profiling
    kill -USR2 $AGENT_PID 2>/dev/null || true
}

# Stop the agent
echo "Stopping agent..."
kill $AGENT_PID 2>/dev/null || true
wait $AGENT_PID 2>/dev/null || true

echo ""
echo "Profile saved to: $PROFILE_OUTPUT"
echo ""
echo "To view the profile:"
echo "  go tool pprof $PROFILE_OUTPUT"
echo ""
echo "To generate a web report:"
echo "  go tool pprof -http=:8080 $PROFILE_OUTPUT"
echo ""
echo "To view top functions:"
echo "  go tool pprof -top $PROFILE_OUTPUT"
