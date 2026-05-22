-- Plan phase batch: idempotent audit bootstrap + checksum load + scoped catalog hit.
-- Executed as one client batch (single round-trip). Parameters: @p1 keys JSON, @p2 scope JSON.

-- Section 1: ensure audit tables (inline minimal; full bootstrap in audit/bootstrap_tables.sql)
IF OBJECT_ID(N'azdo_deploy_meta.history', N'U') IS NULL
BEGIN
    EXEC(N'
CREATE SCHEMA azdo_deploy_meta;
CREATE TABLE azdo_deploy_meta.history (
    id BIGINT IDENTITY(1,1) PRIMARY KEY,
    normalized_key NVARCHAR(512) NOT NULL,
    kind NVARCHAR(64) NOT NULL,
    event NVARCHAR(32) NOT NULL,
    checksum VARBINARY(32) NULL,
    applied_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()
);
');
END;

-- Section 2: checksums (requires @p1)
-- Caller appends load_checksums_openjson body when keys non-empty.
