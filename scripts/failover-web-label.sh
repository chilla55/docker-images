#!/bin/bash
# Failover script: Add web.node=web label to mail when srv1 goes offline
# Run this on a manager node (srv2 or mail)

set -e

SRV1_HOSTNAME="srv1"
MAIL_HOSTNAME="mail"
CHECK_INTERVAL=10  # seconds

echo "[$(date)] Starting web label failover monitor..."

while true; do
    # Get srv1 status
    SRV1_STATUS=$(docker node inspect "$SRV1_HOSTNAME" --format '{{.Status.State}}' 2>/dev/null || echo "unknown")
    
    # Get current web label on mail
    MAIL_WEB_LABEL=$(docker node inspect "$MAIL_HOSTNAME" --format '{{index .Spec.Labels "web.node"}}' 2>/dev/null || echo "")
    
    # Get current web label on srv1
    SRV1_WEB_LABEL=$(docker node inspect "$SRV1_HOSTNAME" --format '{{index .Spec.Labels "web.node"}}' 2>/dev/null || echo "")
    
    if [ "$SRV1_STATUS" != "ready" ]; then
        # srv1 is offline
        if [ "$MAIL_WEB_LABEL" != "web" ]; then
            echo "[$(date)] srv1 is $SRV1_STATUS - Adding web.node=web to mail"
            docker node update --label-add web.node=web "$MAIL_HOSTNAME"
            echo "[$(date)] Failover complete: mail now has web.node=web"
        fi
    else
        # srv1 is online
        if [ "$MAIL_WEB_LABEL" = "web" ] && [ "$SRV1_WEB_LABEL" = "web" ]; then
            echo "[$(date)] srv1 is back online - Removing web.node=web from mail"
            docker node update --label-rm web.node "$MAIL_HOSTNAME"
            echo "[$(date)] Failback complete: web services will migrate back to srv1"
        fi
    fi
    
    sleep "$CHECK_INTERVAL"
done
