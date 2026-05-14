IF NOT EXISTS (SELECT 1 FROM sys.schemas WHERE name = '__migrator') EXEC('CREATE SCHEMA __migrator');

IF OBJECT_ID('__migrator.runs') IS NULL CREATE TABLE __migrator.runs (
    run_id BIGINT IDENTITY(1,1) PRIMARY KEY,
    command VARCHAR(32) NOT NULL,
    started_at DATETIME2 NOT NULL,
    finished_at DATETIME2 NULL,
    result VARCHAR(16) NULL,
    exit_code INT NULL
);

IF OBJECT_ID('__migrator.items') IS NULL CREATE TABLE __migrator.items (
    item_id BIGINT IDENTITY(1,1) PRIMARY KEY,
    run_id BIGINT NOT NULL,
    normalized_key VARCHAR(512) NOT NULL,
    planned_action VARCHAR(64) NOT NULL,
    checksum VARCHAR(64) NOT NULL
);

IF OBJECT_ID('__migrator.attempts') IS NULL CREATE TABLE __migrator.attempts (
    attempt_id BIGINT IDENTITY(1,1) PRIMARY KEY,
    run_id BIGINT NOT NULL,
    item_id BIGINT NOT NULL,
    normalized_key VARCHAR(512) NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    result VARCHAR(16) NOT NULL,
    error_text NVARCHAR(MAX) NULL,
    created_at DATETIME2 NOT NULL
);

IF OBJECT_ID('__migrator.object_state') IS NULL CREATE TABLE __migrator.object_state (
    normalized_key VARCHAR(512) NOT NULL PRIMARY KEY,
    checksum VARCHAR(64) NOT NULL,
    last_attempt_id BIGINT NULL,
    updated_at DATETIME2 NOT NULL
);
