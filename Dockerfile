# syntax=docker/dockerfile:1

# --- build the web UI --------------------------------------------------------
FROM node:24-alpine AS web
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# --- build the API -----------------------------------------------------------
FROM golang:1.26-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/estimeet ./cmd/server

# --- runtime -----------------------------------------------------------------
FROM alpine:3.21
RUN adduser -D -u 10001 estimeet && mkdir -p /data && chown estimeet /data
COPY --from=api /out/estimeet /usr/local/bin/estimeet
COPY --from=web /src/dist /srv/web

USER estimeet
ENV ESTIMEET_ADDR=":8090" \
    ESTIMEET_STATIC_DIR="/srv/web" \
    ESTIMEET_DB_PATH="/data/estimeet.db" \
    ESTIMEET_ENV="production"

# ESTIMEET_SECRET, ESTIMEET_APP_BASE_URL and ESTIMEET_ALLOWED_ORIGINS
# must be supplied at run time.
VOLUME ["/data"]
EXPOSE 8090
ENTRYPOINT ["estimeet"]
