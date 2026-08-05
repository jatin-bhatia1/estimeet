# syntax=docker/dockerfile:1

# Base images are pulled through a prefix instead of by bare name, so the build
# also works on runners where Docker Hub is blocked or rate-limited: pass
# IMAGE_PREFIX=$CI_DEPENDENCY_PROXY_GROUP_IMAGE_PREFIX and every FROM goes
# through the mirror.
ARG IMAGE_PREFIX=docker.io

# --- build the web UI --------------------------------------------------------
FROM ${IMAGE_PREFIX}/library/node:24-alpine AS web
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# --- build the API -----------------------------------------------------------
FROM ${IMAGE_PREFIX}/library/golang:1.26-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/estimeet ./cmd/server

# --- runtime -----------------------------------------------------------------
FROM ${IMAGE_PREFIX}/library/alpine:3.21
RUN adduser -D -u 10001 estimeet && mkdir -p /data && chown estimeet /data
COPY --from=api /out/estimeet /usr/local/bin/estimeet
COPY --from=web /src/dist /srv/web

USER estimeet
# ESTIMEET_ADDR is deliberately unset so that PORT decides, which is how free
# hosts hand a port over. The default here is 8000 rather than the 8090 used in
# development because managed Fargate platforms put an authenticating sidecar in
# front of the container and proxy to 127.0.0.1:8000; a container listening
# anywhere else starts fine but never passes a health check.
ENV ESTIMEET_STATIC_DIR="/srv/web" \
    ESTIMEET_DB_PATH="/data/estimeet.db" \
    ESTIMEET_CONFIG_FILE="/data/estimeet.conf" \
    ESTIMEET_ENV="production" \
    PORT="8000"

# ESTIMEET_SECRET, ESTIMEET_APP_BASE_URL and ESTIMEET_ALLOWED_ORIGINS
# must be supplied at run time.
VOLUME ["/data"]
EXPOSE 8000
ENTRYPOINT ["estimeet"]
