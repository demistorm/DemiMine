# Stage 1: Build Go backend
FROM golang:1.24-alpine AS backend-builder

WORKDIR /build

# Copy Go module files
COPY manager/go.mod ./
COPY manager/go.sum ./
RUN go mod download

# Copy backend source
COPY manager/ ./

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
RUN apk add --no-cache ca-certificates tzdata

# Copy backend binary
COPY --from=backend-builder /demimine /usr/local/bin/demimine

# Copy built frontend
COPY --from=frontend-builder /build/build /app/webui/build

# Create necessary directories
RUN mkdir -p /data /servers /proxies /backups /java

WORKDIR /app

EXPOSE 8080

CMD ["demimine"]
