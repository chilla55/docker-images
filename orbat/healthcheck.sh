#!/bin/bash
# Health check script for Orbat Next.js application

# Check if maintenance mode is active
if [ -f "/tmp/maintenance_active" ]; then
    # During maintenance, we're still "healthy" but serving maintenance page
    exit 0
fi

# Check if the Next.js app is responding
curl -f http://localhost:3000/api/health 2>/dev/null || curl -f http://localhost:3000/ > /dev/null 2>&1

exit $?
