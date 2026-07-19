#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEMP_DIR="$REPO_ROOT/.temp"

echo "Setting up temporary git fixture repository at $TEMP_DIR..."

rm -rf "$TEMP_DIR"
mkdir -p "$TEMP_DIR/sql/dactests/smoke/checks"
mkdir -p "$TEMP_DIR/sql/dactests/smoke/functions"
mkdir -p "$TEMP_DIR/sql/dactests/smoke/indexes"
mkdir -p "$TEMP_DIR/sql/dactests/smoke/procedures"
mkdir -p "$TEMP_DIR/sql/dactests/smoke/tables"
mkdir -p "$TEMP_DIR/sql/dactests/smoke/views"

# Create non-secret config. SQL credentials stay in process environment.
cat << 'EOF' > "$TEMP_DIR/config.toml"
[database]
server = "localhost"
port = 1433
auth = "sql"
encrypt = false
trust_server_certificate = true

[paths]
sql_root = "sql"
sql_base = "sql"

[execution]
log_level = "debug"
EOF

# Create sql files
cat << 'EOF' > "$TEMP_DIR/sql/dactests/smoke/checks/smoke_has_rows.sql"
-- Validation check: ensure smoke_table has rows
IF (SELECT COUNT(*) FROM smoke.smoke_table) = 0
BEGIN
    RAISERROR('smoke_table is empty', 16, 1);
END
EOF

cat << 'EOF' > "$TEMP_DIR/sql/dactests/smoke/functions/get_smoke_rows.sql"
CREATE OR ALTER FUNCTION smoke.get_smoke_rows()
RETURNS TABLE
AS
RETURN
    SELECT id, value, created_at
    FROM smoke.smoke_table;
EOF

cat << 'EOF' > "$TEMP_DIR/sql/dactests/smoke/functions/smoke_count.sql"
CREATE OR ALTER FUNCTION smoke.smoke_count()
RETURNS INT
AS
BEGIN
    DECLARE @c INT;
    SELECT @c = COUNT(*) FROM smoke.smoke_table;
    RETURN @c;
END;
EOF

cat << 'EOF' > "$TEMP_DIR/sql/dactests/smoke/indexes/ix_smoke_table_value.sql"
CREATE UNIQUE NONCLUSTERED INDEX IX_smoke_table_value
    ON smoke.smoke_table (value)
    WHERE value IS NOT NULL;
EOF

cat << 'EOF' > "$TEMP_DIR/sql/dactests/smoke/procedures/refresh_smoke.sql"
CREATE OR ALTER PROCEDURE smoke.refresh_smoke
AS
BEGIN
    SET NOCOUNT ON;

    SELECT
        id,
        value,
        created_at
    FROM smoke.smoke_table
    ORDER BY id;
END;
EOF

cat << 'EOF' > "$TEMP_DIR/sql/dactests/smoke/tables/smoke_table.sql"
CREATE TABLE smoke.smoke_table (
    id INT NOT NULL CONSTRAINT PK_smoke_table PRIMARY KEY,
    value NVARCHAR(100) NULL,
    created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()
);
EOF

cat << 'EOF' > "$TEMP_DIR/sql/dactests/smoke/views/smoke_view.sql"
CREATE OR ALTER VIEW smoke.smoke_view
AS
SELECT
    id,
    value,
    created_at
FROM smoke.smoke_table;
EOF

# Initialize git repository
git -C "$TEMP_DIR" init -b main
git -C "$TEMP_DIR" config user.email "ci@example.com"
git -C "$TEMP_DIR" config user.name "CI Runner"
git -C "$TEMP_DIR" add .
git -C "$TEMP_DIR" commit -m "baseline: dactests/smoke with clean 3-column table"

echo "Temporary git fixture repository successfully initialized!"
