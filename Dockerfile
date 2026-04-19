ARG VERSION=dev
ARG COMMIT=none

# Stage 1: Build Velocity plugins (DemiAuth + DemiDynamic)
FROM eclipse-temurin:21-jdk-noble AS plugin-builder

WORKDIR /plugins

COPY DemiAuth/build.gradle DemiAuth/gradle.properties DemiAuth/settings.gradle* DemiAuth/
COPY DemiAuth/gradle/wrapper DemiAuth/gradle/wrapper/
COPY DemiAuth/gradlew DemiAuth/
COPY DemiAuth/src DemiAuth/src/

COPY DemiDynamic/build.gradle DemiDynamic/gradle.properties DemiDynamic/settings.gradle* DemiDynamic/
COPY DemiDynamic/gradle/wrapper DemiDynamic/gradle/wrapper/
COPY DemiDynamic/gradlew DemiDynamic/
COPY DemiDynamic/src DemiDynamic/src/

RUN cd DemiAuth && chmod +x gradlew && ./gradlew shadowJar --no-daemon && \
    cd /plugins/DemiDynamic && chmod +x gradlew && ./gradlew shadowJar --no-daemon && \
    mkdir -p /output && \
    cp /plugins/DemiAuth/build/libs/DemiAuth-*.jar /output/DemiAuth.jar && \
    cp /plugins/DemiDynamic/build/libs/DemiDynamic-*.jar /output/DemiDynamic.jar

# Stage 2: Build Go backend
FROM golang:1.24-alpine AS backend-builder

ARG VERSION
ARG COMMIT

WORKDIR /build

COPY manager/go.mod ./
COPY manager/go.sum ./
RUN go mod download

COPY manager/ ./

RUN mkdir -p resources
COPY --from=plugin-builder /output/DemiAuth.jar resources/DemiAuth.jar
COPY --from=plugin-builder /output/DemiDynamic.jar resources/DemiDynamic.jar

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=$VERSION -X main.commit=$COMMIT" \
    -o /demimine ./cmd/server

# Stage 3: Build SvelteKit frontend
FROM node:20-alpine AS frontend-builder

WORKDIR /build

COPY webui/package.json ./
COPY webui/package-lock.json ./
COPY webui/svelte.config.js ./
COPY webui/vite.config.ts ./
COPY webui/tailwind.config.js ./
COPY webui/postcss.config.js ./
COPY webui/tsconfig.json ./

RUN npm ci

COPY webui/src ./src
COPY webui/static ./static

RUN npm run build

# Stage 4: Final runtime image
FROM alpine:3.19

LABEL maintainer="demimine"
LABEL version=${VERSION}
LABEL commit=${COMMIT}

RUN apk add --no-cache ca-certificates tzdata zstd

COPY --from=backend-builder /demimine /usr/local/bin/demimine
COPY --from=frontend-builder /build/build /app/webui/build
COPY --from=backend-builder /build/resources /app/resources

RUN mkdir -p /data /servers /proxies /backups /java

WORKDIR /app

EXPOSE 8080

STOPSIGNAL SIGTERM

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

CMD ["demimine"]
