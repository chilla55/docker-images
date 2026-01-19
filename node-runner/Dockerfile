FROM golang:1.21-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN go build -o /entrypoint main.go

FROM node:20-alpine

ARG UID=1000
ARG GID=1000
ARG USERNAME=nodeapp

RUN apk add --no-cache git curl netcat-openbsd tzdata \
  && (addgroup -S -g ${GID} ${USERNAME} 2>/dev/null || addgroup -S ${USERNAME}) \
  && (adduser -S -D -G ${USERNAME} -u ${UID} ${USERNAME} 2>/dev/null || adduser -S -D -G ${USERNAME} ${USERNAME}) \
  && mkdir -p /workspace \
  && chown ${USERNAME}:${USERNAME} /workspace

WORKDIR /workspace

ENV APP_DIR=/workspace \
    NODE_ENV=production

COPY --from=builder /entrypoint /entrypoint

USER ${USERNAME}

ENTRYPOINT ["/entrypoint"]
