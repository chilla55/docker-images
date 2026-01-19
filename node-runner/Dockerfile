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
  && addgroup -g ${GID} ${USERNAME} \
  && adduser -D -G ${USERNAME} -u ${UID} ${USERNAME} \
  && mkdir -p /workspace \
  && chown ${USERNAME}:${USERNAME} /workspace

WORKDIR /workspace

ENV APP_DIR=/workspace \
    NODE_ENV=production

COPY --from=builder /entrypoint /entrypoint

USER ${USERNAME}

ENTRYPOINT ["/entrypoint"]
