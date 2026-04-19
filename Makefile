.PHONY: build run test lint clean docker-build docker-run

build:
	go build -o bin/server ./cmd/server

run:
	go run ./cmd/server

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
