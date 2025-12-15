-- ClickHouse Bootstrap - runs AFTER cluster coordination is ready
-- This script must be executed manually after cluster is up:
--   Local dev: task bootstrap-clickhouse
--   Production: Kubernetes Job with Vault template substitution

-- Create database across cluster
CREATE DATABASE IF NOT EXISTS analytics ON CLUSTER doujins;

-- Create analytics user with password from Vault
-- Production uses Vault template: {{- with secret "kv-prod/data/infra/clickhouse" -}}{{ .Data.data.password }}{{- end -}}
-- Local dev uses: analytics_password
CREATE USER IF NOT EXISTS analytics_user ON CLUSTER doujins
  IDENTIFIED WITH plaintext_password BY '{{CLICKHOUSE_PASSWORD}}'
  HOST ANY;

-- Grant full database access WITH GRANT OPTION so user can run migrations
GRANT ON CLUSTER doujins ALL ON analytics.* TO analytics_user WITH GRANT OPTION;

-- Grant system table permissions for monitoring and debugging
GRANT ON CLUSTER doujins SELECT ON system.tables TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.columns TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.databases TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.parts TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.parts_columns TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.mutations TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.merges TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.processes TO analytics_user;
