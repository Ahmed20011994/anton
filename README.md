# Anton

Anton is a Go backend service built with the standard library.

## Run Locally

```bash
make run
```

The server starts on port 8080 by default. Override with `PORT=<n>`.

## Run via Docker

```bash
make docker-build
make docker-run
```

Copy `.env.example` to `.env` and adjust values before running the container.

## Endpoints

| Method | Path      | Description       |
|--------|-----------|-------------------|
| GET    | /healthz  | Health check      |

## Project Structure

```
cmd/server/        # Entrypoint — wires dependencies, starts HTTP server
internal/
  config/          # Environment-based configuration
  handler/         # HTTP handlers
  middleware/       # HTTP middleware (logging)
migrations/        # SQL migration files
scripts/           # Utility scripts
```

## Make Targets

| Target              | Description                        |
|---------------------|------------------------------------|
| `make build`        | Compile to `bin/server`            |
| `make run`          | Run server via `go run`            |
| `make test`         | Run all tests with `-v -race`      |
| `make lint`         | Run `golangci-lint`                |
| `make clean`        | Remove `bin/`                      |
| `make docker-build` | Build Docker image `anton:latest`  |
| `make docker-run`   | Run container on port 8080         |
