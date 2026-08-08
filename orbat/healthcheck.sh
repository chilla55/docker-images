#!/bin/bash
# Health check script for Orbat Next.js application

# The Go entrypoint starts a maintenance server before cloning/building the app.
# Treat either the application or that server as healthy so Docker Swarm does
# not terminate a legitimate, long-running Next.js production build.
APP_PORT="${PORT:-3000}"
MAINTENANCE_PORT="${MAINTENANCE_PORT:-3001}"

curl -fsS "http://localhost:${APP_PORT}/api/health" >/dev/null 2>&1 || \
curl -fsS "http://localhost:${APP_PORT}/" >/dev/null 2>&1 || \
curl -fsS "http://localhost:${MAINTENANCE_PORT}/healthcheck" >/dev/null 2>&1
