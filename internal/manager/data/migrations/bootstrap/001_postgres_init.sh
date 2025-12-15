#!/bin/bash
set -e

# PostgreSQL initialization script - runs on first container startup
echo "Running PostgreSQL bootstrap (schemas/extensions)..."

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -f "/tmp/postgres-init.sql"

echo "PostgreSQL bootstrap complete"
