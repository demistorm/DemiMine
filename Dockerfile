# Stage 1: Build Go backend
FROM golang:1.24-alpine AS backend-builder

WORKDIR /build

# Copy Go module files
COPY manager/go.mod ./
COPY manager/go.sum ./
RUN go mod download

# Copy backend source
COPY manager/ ./

# Copy DemiAuth plugin for Velocity proxies
COPY DemiAuth/build/libs/DemiAuth-1.0.0.jar manager/resources/
# Copy DemiDynamic plugin for Velocity proxies
COPY DemiDynamic/build/libs/DemiDynamic-1.0.3.jar manager/resources/

# Build backend
RUN CGO_ENABLED=0 GOOS=linux go build -o /demimine ./cmd/server

# Stage 2: Build SvelteKit frontend
FROM node:20-alpine AS frontend-builder

WORKDIR /build

# Copy frontend files
COPY webui/package.json ./
COPY webui/svelte.config.js ./
COPY webui/vite.config.ts ./
COPY webui/tailwind.config.js ./
COPY webui/postcss.config.js ./
COPY webui/tsconfig.json ./

# Install dependencies
RUN npm install

# Copy frontend source
COPY webui/src ./src

# Build frontend
RUN npm run build

# Stage 3: Final runtime image
FROM alpine:3.19

# Install necessary packages
RUN apk add --no-cache ca-certificates tzdata zstd

# Copy backend binary
COPY --from=backend-builder /demimine /usr/local/bin/demimine

# Copy built frontend
COPY --from=frontend-builder /build/build /app/webui/build

# Copy resources (DemiAuth plugin)
COPY --from=backend-builder /build/resources /app/resources

# Create necessary directories
RUN mkdir -p /data /servers /proxies /backups /java

WORKDIR /app

EXPOSE 8080

STOPSIGNAL SIGTERM

CMD ["demimine"]
