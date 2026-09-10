# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32
FROM node:26-alpine@sha256:ef24c5053d50fdc3e4e56eb4e7ddb7861874ab0fdc797046ba897581deb8e868 AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27.1-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
ARG VERSION=dev
ARG COMMIT_SHA=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT_SHA} -X main.built=${BUILD_TIME}" -o /out/velora-dns ./cmd/server

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
RUN addgroup -g 10001 velora && adduser -D -H -u 10001 -G velora velora && mkdir /data && chown velora:velora /data
WORKDIR /app
COPY --from=backend /out/velora-dns /usr/local/bin/velora-dns
COPY --from=web /src/web/dist /app/web/dist
ENV VELORA_DNS_LISTEN=0.0.0.0:5353 VELORA_HTTP_LISTEN=0.0.0.0:8080 VELORA_DATABASE_PATH=/data/velora.db
USER 10001:10001
EXPOSE 5353/udp 5353/tcp 8080/tcp
VOLUME ["/data"]
HEALTHCHECK --interval=15s --timeout=4s --start-period=10s --retries=3 CMD ["velora-dns", "-healthcheck", "http://127.0.0.1:8080/ready"]
ENTRYPOINT ["velora-dns"]
