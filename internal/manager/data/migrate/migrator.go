package migrate

import (
	"context"
	"fmt"

	authkitmigrations "github.com/PaulFidika/authkit/migrations/postgres"
	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/manager/data/migrations/clickhouse"
	"github.com/doujins-org/doujins-billing/internal/manager/data/migrations/postgres"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/migratekit"

	"github.com/jackc/pgx/v5/pgxpool"
	riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

// RunAuthKit applies only AuthKit migrations to the profiles schema.
// Run this once before running Run() on all services.
func RunAuthKit(ctx context.Context, cfg *config.Config) error {
	if cfg == nil || cfg.DB == nil {
		return fmt.Errorf("missing database config")
	}

	database, err := db.NewDB(cfg.DB)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = database.Close() }()

	bunDB, ok := database.GetDB().(*bun.DB)
	if !ok {
		return fmt.Errorf("unexpected db type for migrator")
	}
	sqlDB := bunDB.DB

	log.Info("Running AuthKit migrations (profiles schema)...")
	migrations, err := migratekit.LoadFromFS(authkitmigrations.FS)
	if err != nil {
		return fmt.Errorf("authkit: load migrations: %w", err)
	}

	m := migratekit.NewPostgres(sqlDB, "authkit")
	// ApplyMigrations now calls Setup() automatically within the lock
	if err := m.ApplyMigrations(ctx, migrations); err != nil {
		return fmt.Errorf("authkit: apply migrations: %w", err)
	}

	log.Info("✓ AuthKit migrations completed successfully")
	return nil
}

// RunPostgres applies all Postgres migrations:
// 0. AuthKit (profiles schema) - via migratekit
// 1. River (billing schema) - via rivermigrate
// 2. Billing (billing schema) - via migratekit
func RunPostgres(ctx context.Context, cfg *config.Config) error {
	if cfg == nil || cfg.DB == nil {
		return fmt.Errorf("missing database config")
	}

	// ---------- 0. AuthKit Migrations (profiles schema) ----------
	log.Info("Running AuthKit migrations (profiles schema)...")
	if err := RunAuthKit(ctx, cfg); err != nil {
		return fmt.Errorf("authkit migrations failed: %w", err)
	}

	database, err := db.NewDB(cfg.DB)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = database.Close() }()

	bunDB, ok := database.GetDB().(*bun.DB)
	if !ok {
		return fmt.Errorf("unexpected db type for migrator")
	}
	sqlDB := bunDB.DB

	schema := "billing" // Hardcoded schema

	// ---------- 1. River Migrations (billing schema) ----------
	log.Info("Running River migrations (billing schema)...")
	if err := runRiverMigrations(ctx, cfg, schema); err != nil {
		return fmt.Errorf("river migrations failed: %w", err)
	}

	// ---------- 2. Billing Migrations (billing schema) ----------
	log.Info("Running Billing migrations (billing schema)...")
	migrations, err := migratekit.LoadFromFS(postgresmigrations.FS)
	if err != nil {
		return fmt.Errorf("billing: load migrations: %w", err)
	}

	m := migratekit.NewPostgres(sqlDB, "billing")
	// ApplyMigrations now calls Setup() automatically within the lock
	if err := m.ApplyMigrations(ctx, migrations); err != nil {
		return fmt.Errorf("billing: apply migrations: %w", err)
	}
	log.Info("✓ Billing migrations completed successfully")
	return nil
}

// RunClickHouse applies ClickHouse migrations
func RunClickHouse(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("missing config")
	}

	if cfg.ClickHouse == nil || cfg.ClickHouse.ClientAddr == "" {
		log.Info("ClickHouse not configured; skipping ClickHouse migrations")
		return nil
	}

	log.Infof("Running ClickHouse migrations... %v+", cfg.ClickHouse)
	if err := runClickHouseMigrations(ctx, cfg.ClickHouse); err != nil {
		return fmt.Errorf("clickhouse migrations failed: %w", err)
	}

	log.Info("✓ ClickHouse migrations completed successfully")
	return nil
}

// Run applies all migrations (Postgres and ClickHouse independently):
// Postgres: AuthKit → River → Billing
// ClickHouse: Billing analytics
func Run(ctx context.Context, cfg *config.Config) error {
	if cfg == nil || cfg.DB == nil {
		return fmt.Errorf("missing database config")
	}

	// Run Postgres migrations
	pgErr := RunPostgres(ctx, cfg)

	// Run ClickHouse migrations independently (don't stop on Postgres failure)
	chErr := RunClickHouse(ctx, cfg)

	// Report results
	if pgErr != nil && chErr != nil {
		return fmt.Errorf("both migrations failed: postgres=%v; clickhouse=%v", pgErr, chErr)
	}
	if pgErr != nil {
		return pgErr
	}
	if chErr != nil {
		return chErr
	}

	log.Info("✓ All migrations completed successfully")
	return nil
}

// runRiverMigrations executes River's built-in schema migrations
func runRiverMigrations(ctx context.Context, cfg *config.Config, schema string) error {
	pgxPool, err := pgxpool.New(ctx, cfg.DB.GetConnectionString())
	if err != nil {
		return fmt.Errorf("create pgx pool: %w", err)
	}
	defer pgxPool.Close()

	riverCfg := &rivermigrate.Config{}
	if schema != "" && schema != "public" {
		riverCfg.Schema = schema
	}

	// migrator, err := rivermigrate.New(riverpgxv5.New(pgxPool).(riverdriver.Driver[]), riverCfg)
	migrator, err := rivermigrate.New(riverpgxv5.New(pgxPool), riverCfg)
	if err != nil {
		return fmt.Errorf("create River migrator: %w", err)
	}

	res, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("run River migrations: %w", err)
	}

	if len(res.Versions) == 0 {
		log.Info("No new River migrations to apply")
	} else {
		log.Infof("Applied %d River migration(s)", len(res.Versions))
	}

	return nil
}

// runClickHouseMigrations applies ClickHouse migrations using migratekit
func runClickHouseMigrations(ctx context.Context, cfg *config.ClickHouseConfig) error {
	chDB := cfg.Database
	if chDB == "" {
		chDB = "analytics"
	}

	chCluster := cfg.Cluster
	if chCluster == "" {
		chCluster = "doujins"
	}

	chMigrations, err := migratekit.LoadFromFS(clickhousemigrations.FS)
	if err != nil {
		return fmt.Errorf("clickhouse: load migrations: %w", err)
	}

	m := migratekit.NewClickHouse(&migratekit.ClickHouseConfig{
		ClientAddr: cfg.ClientAddr,
		Database:   chDB,
		Username:   cfg.Username,
		Password:   cfg.Password,
		App:        "billing",
		Cluster:    chCluster,
	})
	// ApplyMigrations now calls Setup() automatically within the lock
	if err := m.ApplyMigrations(ctx, chMigrations); err != nil {
		return fmt.Errorf("clickhouse: apply migrations: %w", err)
	}

	log.Info("✓ ClickHouse migrations completed successfully")
	return nil
}
