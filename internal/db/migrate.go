package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ahmed20011994/anton/migrations"
)

const schemaMigrationsDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    filename    TEXT PRIMARY KEY,
    sha256      TEXT NOT NULL,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, schemaMigrationsDDL); err != nil {
		return fmt.Errorf("Migrate: ensure schema_migrations: %w", err)
	}

	files, err := listSQLFiles(migrations.FS)
	if err != nil {
		return fmt.Errorf("Migrate: list files: %w", err)
	}

	applied, err := loadApplied(ctx, pool)
	if err != nil {
		return fmt.Errorf("Migrate: load applied: %w", err)
	}

	for _, name := range files {
		body, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return fmt.Errorf("Migrate: read %s: %w", name, err)
		}
		hash := sha256Hex(body)

		if prev, ok := applied[name]; ok {
			if prev != hash {
				return fmt.Errorf("Migrate: %s hash drift: applied=%s current=%s — never modify an applied migration", name, prev, hash)
			}
			continue
		}

		if err := applyOne(ctx, pool, name, body, hash); err != nil {
			return fmt.Errorf("Migrate: apply %s: %w", name, err)
		}
	}
	return nil
}

func applyOne(ctx context.Context, pool *pgxpool.Pool, name string, body []byte, hash string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("applyOne: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, string(body)); err != nil {
		return fmt.Errorf("applyOne: exec %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (filename, sha256) VALUES ($1, $2)`,
		name, hash,
	); err != nil {
		return fmt.Errorf("applyOne: record %s: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("applyOne: commit %s: %w", name, err)
	}
	return nil
}

func listSQLFiles(efs fs.FS) ([]string, error) {
	var out []string
	err := fs.WalkDir(efs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".sql") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listSQLFiles: walk: %w", err)
	}
	sort.Strings(out)
	return out, nil
}

func loadApplied(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	rows, err := pool.Query(ctx, `SELECT filename, sha256 FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("loadApplied: query: %w", err)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var name, hash string
		if err := rows.Scan(&name, &hash); err != nil {
			return nil, fmt.Errorf("loadApplied: scan: %w", err)
		}
		out[name] = hash
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("loadApplied: rows: %w", err)
	}
	return out, nil
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
