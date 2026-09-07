.PHONY: dev backend frontend install go-tools test lint build check docker-build docker-up release-test

install:
	npm --prefix web ci

go-tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2

backend:
	go run ./cmd/server

frontend:
	npm --prefix web run dev

dev: docker-up

test:
	go test -race ./cmd/... ./internal/... ./tests/...
	npm --prefix web test -- --run

lint:
	test -z "$$(gofmt -l cmd internal tests)"
	go vet ./cmd/... ./internal/... ./tests/...
	golangci-lint run ./cmd/... ./internal/... ./tests/...
	npm --prefix web run lint
	npm --prefix web run typecheck

build:
	npm --prefix web run build
	CGO_ENABLED=0 go build -trimpath -o bin/velora-dns ./cmd/server

check: lint test build release-test

release-test:
	SKIP_REMOTE_CHECK=true ./scripts/validate-release.sh RELEASE
	./scripts/test-release.sh
	./scripts/test-release-notes.sh
	./scripts/test-release-announcement.sh
	python3 scripts/test-weekly-growth-report.py
	./scripts/test-github-workflows.sh
	python3 scripts/check-markdown-links.py

docker-build:
	docker build -t velora-dns:local .

docker-up:
	docker compose up --build -d
