package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Ahmed20011994/anton/internal/config"
	"github.com/Ahmed20011994/anton/internal/db"
	"github.com/Ahmed20011994/anton/internal/repository"
)

func main() {
	slug := flag.String("slug", "", "tenant slug (required, e.g. acme)")
	name := flag.String("name", "", "human-readable name (defaults to slug)")
	flag.Parse()

	if *slug == "" {
		fmt.Fprintln(os.Stderr, "usage: seed-tenant -slug <slug> [-name <name>]")
		os.Exit(2)
	}
	if *name == "" {
		*name = *slug
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db pool", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		logger.Error("migrate", "err", err)
		os.Exit(1)
	}

	apiKeyBytes := make([]byte, 32)
	if _, err := rand.Read(apiKeyBytes); err != nil {
		logger.Error("generate api key", "err", err)
		os.Exit(1)
	}
	apiKey := base64.RawURLEncoding.EncodeToString(apiKeyBytes)

	hash, err := bcrypt.GenerateFromPassword([]byte(apiKey), bcrypt.DefaultCost)
	if err != nil {
		logger.Error("bcrypt hash", "err", err)
		os.Exit(1)
	}

	repo := repository.NewTenantRepo(pool)
	t, err := repo.Create(ctx, *slug, *name, string(hash))
	if err != nil {
		logger.Error("create tenant", "err", err)
		os.Exit(1)
	}

	fmt.Println("─────────────────────────────────────────────────────────────")
	fmt.Println("Tenant created. The API key below is shown ONCE — save it.")
	fmt.Println("─────────────────────────────────────────────────────────────")
	fmt.Printf("  tenant_id : %s\n", t.ID)
	fmt.Printf("  slug      : %s\n", t.Slug)
	fmt.Printf("  name      : %s\n", t.Name)
	fmt.Printf("  api_key   : %s\n", apiKey)
	fmt.Println("─────────────────────────────────────────────────────────────")
	fmt.Println("Use it via the X-Anton-Key header on /v1/tenants/" + t.Slug + "/* requests.")
}
