# Anton — Claude Code Guidelines

## Project
- **Name**: Anton
- **Module**: `github.com/Ahmed20011994/anton`

## Architecture
Handler → Service → Repository

The service and repository layers do not exist yet. When adding them:
- Handlers call services, never repositories directly.
- Services hold business logic and call repositories for persistence.
- Repositories are the only layer that touches the database.

## Coding Rules

**Error wrapping** — always wrap with context:
```go
if err != nil {
    return fmt.Errorf("functionName: %w", err)
}
```

**No global state** — inject all dependencies via constructors.

**Context first** — any function that does I/O takes `ctx context.Context` as its first parameter.

**Tests** — table-driven only. No single-case test functions.

**Linting** — run `make lint` before considering any task done.

## Make Targets

| Target         | Description                              |
|----------------|------------------------------------------|
| `make build`   | Compile to `bin/server`                  |
| `make run`     | Run server via `go run`                  |
| `make test`    | Run all tests with `-v -race`            |
| `make lint`    | Run `golangci-lint`                      |
| `make clean`   | Remove `bin/`                            |
| `make docker-build` | Build Docker image `anton:latest`   |
| `make docker-run`   | Run container on port 8080          |
