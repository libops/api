#!/usr/bin/env bash

set -eou pipefail

TARGET="${1:-smtp.relay.internal}"

echo "Configuring Host for Transparent SMTP Interception..."
echo "Target: $TARGET"

echo "Waiting for $TARGET to resolve..."
MAX_TRIES=10
COUNT=0
while [ $COUNT -lt $MAX_TRIES ]; do
    PSC_IP=$(dig +short "$TARGET" | head -n1)
    if [ -n "$PSC_IP" ]; then
        echo "Successfully resolved $TARGET to $PSC_IP"
        break
    fi
    echo "Still waiting for DNS... (Try $((COUNT+1))/$MAX_TRIES)"
    sleep 5
    COUNT=$((COUNT+1))
done

if [ -z "$PSC_IP" ]; then
    # Fallback: if it didn't resolve, and input looks like an IP, use it.
    if [[ "$TARGET" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        PSC_IP="$TARGET"
        echo "Using literal IP: $PSC_IP"
    else
        echo "Error: Could not resolve $TARGET and it is not a valid IP."
        exit 1
    fi
fi

sysctl -w net.ipv4.ip_forward=1
grep -qF "net.ipv4.ip_forward=1" /etc/sysctl.conf || echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf

echo "Applying NAT Redirect rules for $PSC_IP..."
iptables -t nat -D PREROUTING -i docker0 -p tcp --dport 25 -j DNAT --to-destination "$PSC_IP" 2>/dev/null || true
iptables -t nat -A PREROUTING -i docker0 -p tcp --dport 25 -j DNAT --to-destination "$PSC_IP"

echo "Applying Safety Rate Limits (20/hour)..."
iptables -D FORWARD -p tcp --dport 25 -d "$PSC_IP" -j DROP 2>/dev/null || true
iptables -D FORWARD -p tcp --dport 25 -d "$PSC_IP" -j ACCEPT 2>/dev/null || true
iptables -A FORWARD -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
iptables -A FORWARD -p tcp --dport 25 -d "$PSC_IP" -m conntrack --ctstate NEW \
    -m hashlimit \
    --hashlimit-name smtp_ratelimit \
    --hashlimit-mode srcip \
    --hashlimit-above 20/hour \
    --hashlimit-burst 10 \
    -j DROP
iptables -A FORWARD -p tcp --dport 25 -d "$PSC_IP" -j ACCEPT


echo "----------------------------------------------------------------"
echo "Setup Complete! Any Docker container sending to Port 25 is now"
echo "intercepted and sent to the LibOps Relay via $PSC_IP."
echo "----------------------------------------------------------------"
