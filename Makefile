# PU1 — helpers
.PHONY: up down logs dev test test-db eval build clean

up:            ## Levantar el stack completo (build + detached)
	docker compose up --build -d

down:          ## Detener y eliminar el stack
	docker compose down

logs:          ## Ver logs del API
	docker compose logs -f api

build:         ## Build local sin compose (backend + frontend)
	cd backend && go build ./...
	cd frontend && npm run build

test:          ## Unit tests (domain, engine, evaluación) y vet
	cd backend && go vet ./... && go test ./internal/domain/ ./internal/engine/ ./internal/features/evaluation/ -count=1

test-db:       ## Tests de integración contra PostgreSQL efímero (todas las features)
	docker rm -f pu1-testdb 2>/dev/null || true
	docker run --rm -d --name pu1-testdb -p 5433:5432 \
		-e POSTGRES_PASSWORD=test -e POSTGRES_USER=test -e POSTGRES_DB=pu1test \
		postgres:17-alpine
	sleep 4
	cd backend && TEST_DATABASE_URL='postgres://test:test@127.0.0.1:5433/pu1test?sslmode=disable' \
		go test ./... -count=1
	docker rm -f pu1-testdb

eval:          ## Comparación base vs. propuesto por CLI
	cd backend && go run ./cmd/evaluate

dev:           ## Desarrollo: solo db+api en docker, vite en local
	docker compose -f docker-compose.yml -f compose.dev.yml up --build -d db api
	@echo "API en http://localhost:8080 — ahora: cd frontend && VITE_API_TARGET=http://localhost:8080 npm run dev"
