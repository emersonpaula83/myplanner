.PHONY: dev db db-stop migrate-up migrate-down build test

# Database
db:
	docker compose up -d db

db-stop:
	docker compose down

db-logs:
	docker compose logs -f db

# Backend
dev:
	cd backend && go run ./cmd/api

build:
	cd backend && go build -o bin/api ./cmd/api

test:
	cd backend && go test ./...

# Migrations
migrate-up:
	cd backend && go run ./cmd/migrate -direction up

migrate-down:
	cd backend && go run ./cmd/migrate -direction down

seed:
	cd backend && go run ./cmd/seed

# Frontend
frontend-dev:
	cd frontend && npm run dev

frontend-build:
	cd frontend && npm run build
