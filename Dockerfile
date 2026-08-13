# syntax=docker/dockerfile:1

# --- Build stage ---------------------------------------------------------
FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# modernc.org/sqlite is pure Go (no cgo), so CGO_ENABLED=0 gives a fully
# static binary that runs on the scratch-derived runtime image below.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/jira-home .

# --- Runtime stage --------------------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates && \
    addgroup -S app && adduser -S app -G app && \
    mkdir -p /data && chown app:app /data

COPY --from=builder /out/jira-home /app/jira-home

USER app
WORKDIR /app

ENV JIRA_HOME_DB=/data/jira-home.db
ENV JIRA_HOME_ADDR=:8080

EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/app/jira-home"]
