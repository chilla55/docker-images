# Orbat Next.js Application Container
# Clones/pulls from GitHub and builds on startup
FROM golang:1.21-alpine AS builder

WORKDIR /build
COPY main.go .
RUN go build -o entrypoint .

FROM node:20-alpine

# Install dependencies only when needed
RUN apk add --no-cache git curl bash

WORKDIR /app

# Copy maintenance page
COPY maintenance.html /maintenance.html

# Copy Go entrypoint binary from builder
COPY --from=builder /build/entrypoint /entrypoint
RUN chmod +x /entrypoint

# Copy healthcheck script
COPY healthcheck.sh /healthcheck.sh
RUN chmod +x /healthcheck.sh

EXPOSE 3000

ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000

HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD ["/healthcheck.sh"]

ENTRYPOINT ["/entrypoint"]
