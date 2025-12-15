-- PostgreSQL Bootstrap
-- Simple initialization: create schemas and install extensions
-- All apps use the admin super-user

-- Create schemas
CREATE SCHEMA IF NOT EXISTS doujins;
CREATE SCHEMA IF NOT EXISTS billing;
CREATE SCHEMA IF NOT EXISTS hentai0;
CREATE SCHEMA IF NOT EXISTS profiles;

-- Drop extensions if they exist in wrong schema and recreate in public
-- This fixes issues where extensions were created in app schemas instead of public
DROP EXTENSION IF EXISTS pgcrypto CASCADE;
DROP EXTENSION IF EXISTS citext CASCADE;
DROP EXTENSION IF EXISTS vector CASCADE;
DROP EXTENSION IF EXISTS pg_trgm CASCADE;
DROP EXTENSION IF EXISTS btree_gist CASCADE;

-- Install required extensions in public schema
-- Explicitly specify schema to ensure they're accessible from all app schemas
CREATE EXTENSION pgcrypto WITH SCHEMA public;
CREATE EXTENSION citext WITH SCHEMA public;
CREATE EXTENSION vector WITH SCHEMA public;
CREATE EXTENSION pg_trgm WITH SCHEMA public;
CREATE EXTENSION btree_gist WITH SCHEMA public;

-- Create migratekit migrations tracking table
-- Note: Locking is now handled via PostgreSQL advisory locks, not this table
CREATE TABLE IF NOT EXISTS public.migrations (
    id BIGSERIAL PRIMARY KEY,
    app TEXT NOT NULL,
    database TEXT NOT NULL,
    name TEXT NOT NULL,
    migrated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(app, database, name)
);
