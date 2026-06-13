# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS registry-builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /build

COPY go.mod go.sum* ./
RUN go mod download

COPY main.go ./

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
	go build -trimpath -ldflags="-s -w" -o /out/pelican-registry-agent .

FROM ghcr.io/pelican-dev/panel:latest

LABEL maintainer="chilla55" \
	org.opencontainers.image.title="Pelican Panel" \
	org.opencontainers.image.description="Official Pelican image with local Caddy registry integration" \
	org.opencontainers.image.source="https://github.com/chilla55/docker-images"

USER root

COPY --from=registry-builder /out/pelican-registry-agent /usr/local/bin/pelican-registry-agent
COPY supervisord.conf /tmp/pelican-registry-agent.supervisord.conf
COPY entrypoint-wrapper.sh /usr/local/bin/pelican-entrypoint-wrapper.sh

RUN chmod 0755 /usr/local/bin/pelican-registry-agent /usr/local/bin/pelican-entrypoint-wrapper.sh && \
	grep -q '^\[program:registry-agent\]$' /etc/supervisord.conf || { \
		printf '\n' >> /etc/supervisord.conf && \
		cat /tmp/pelican-registry-agent.supervisord.conf >> /etc/supervisord.conf; \
	} && \
	rm -f /tmp/pelican-registry-agent.supervisord.conf

USER www-data

ENTRYPOINT ["/bin/ash", "/usr/local/bin/pelican-entrypoint-wrapper.sh"]
CMD ["supervisord", "-n", "-c", "/etc/supervisord.conf"]
