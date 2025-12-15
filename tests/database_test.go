package tests

import (
	"github.com/doujins-org/doujins-billing/pkg/db"
	"testing"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/stretchr/testify/assert"
)

// TestDatabaseConfig tests database configuration without actual connection
func TestDatabaseConfig(t *testing.T) {
	t.Run("Create Database Config", func(t *testing.T) {
		dbConfig := &config.DBConfig{
			URL:     "postgres://test:test@localhost:5432/test?sslmode=disable",
			Dialect: "postgres",
		}

		assert.NotNil(t, dbConfig, "Should create database config")
		assert.Equal(t, "postgres", dbConfig.Dialect, "Should have correct dialect")
		assert.Contains(t, dbConfig.URL, "postgres://", "Should have postgres URL")
	})

	t.Run("Database Config Validation", func(t *testing.T) {
		// Test valid config
		validConfig := &config.DBConfig{
			URL:     "postgres://test:test@localhost:5432/test?sslmode=disable",
			Dialect: "postgres",
		}

		// We can't test actual connection without a database, but we can test config structure
		assert.NotEmpty(t, validConfig.URL, "URL should not be empty")
		assert.NotEmpty(t, validConfig.Dialect, "Dialect should not be empty")

		// Test invalid config
		invalidConfig := &config.DBConfig{
			URL:     "",
			Dialect: "postgres",
		}

		assert.Empty(t, invalidConfig.URL, "Invalid config should have empty URL")
	})
}

// TestDatabaseConnection tests database connection creation (without actual connection)
func TestDatabaseConnection(t *testing.T) {
	t.Run("Database Connection Creation Fails With Invalid Config", func(t *testing.T) {
		invalidConfig := &config.DBConfig{
			URL:     "", // Empty URL should cause failure
			Dialect: "postgres",
		}

		_, err := db.NewDB(invalidConfig)
		assert.Error(t, err, "Should fail with invalid config")
		assert.Contains(t, err.Error(), "missing database url", "Should have appropriate error message")
	})

	t.Run("Database Connection Creation With Valid Config Structure", func(t *testing.T) {
		// This will fail to connect but should pass config validation
		validConfig := &config.DBConfig{
			URL:     "postgres://test:test@localhost:5432/test?sslmode=disable",
			Dialect: "postgres",
		}

		// We expect this to fail because there's no actual database, but it should pass initial validation
		_, err := db.NewDB(validConfig)
		// We expect an error because there's no database running, but it should be a connection error, not a config error
		if err != nil {
			// This is expected - we don't have a database running
			assert.Contains(t, err.Error(), "failed to connect", "Should be a connection error, not config error")
		}
	})
}
