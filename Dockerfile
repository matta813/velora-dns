# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32
FROM node:26-alpine@sha256:2d984a15c9b54fd0aeb608b8e0d0d83529eb34d2966db27a1fb4f1edc3d298a3 AS web
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

FROM alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40
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
