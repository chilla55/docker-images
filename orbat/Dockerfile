# Orbat Next.js Application Container
# Clones/pulls from GitHub and builds on startup
FROM node:20-alpine AS base

# Install dependencies only when needed
RUN apk add --no-cache git curl bash socat

WORKDIR /app

# Copy maintenance page
COPY maintenance.html /maintenance.html

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Copy healthcheck script
COPY healthcheck.sh /healthcheck.sh
RUN chmod +x /healthcheck.sh

EXPOSE 3000

ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000

HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD ["/healthcheck.sh"]

ENTRYPOINT ["/entrypoint.sh"]
