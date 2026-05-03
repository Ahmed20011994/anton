.PHONY: build run test lint clean docker-build docker-run migrate sync seed-tenant compose-up compose-down

build:
	go build -o bin/server ./cmd/server

run:
	go run ./cmd/server

migrate:
	go run ./cmd/migrate

sync:
	go run ./cmd/sync

seed-tenant:
	@if [ -z "$(SLUG)" ]; then echo "usage: make seed-tenant SLUG=acme [NAME=Acme]"; exit 2; fi
	go run ./cmd/seed-tenant -slug $(SLUG) $(if $(NAME),-name "$(NAME)",)

compose-up:
	docker compose up -d postgres

compose-down:
	docker compose down

test:
	go test ./... -v -race

lint:
	golangci-lint run ./...

ifeq ($(OS),Windows_NT)
    RM = if exist bin (rmdir /s /q bin)
else
    RM = rm -rf bin/
endif

clean:
	$(RM)

docker-build:
	docker build -t anton:latest .

docker-run:
	docker run --rm -p 8080:8080 --env-file .env anton:latest
