BEGIN TRY
    IF SCHEMA_ID('azdo_deploy_meta') IS NULL
        EXEC('CREATE SCHEMA azdo_deploy_meta');
END TRY
BEGIN CATCH
    IF ERROR_NUMBER() NOT IN (2714, 2759) THROW;
END CATCH;

BEGIN TRY
    IF OBJECT_ID('azdo_deploy_meta.history') IS NULL
    CREATE TABLE azdo_deploy_meta.history (
        id              BIGINT IDENTITY(1,1) PRIMARY KEY,
        normalized_key  NVARCHAR(512) NOT NULL,
        kind            VARCHAR(16)  NOT NULL,
        checksum        VARCHAR(64)  NOT NULL,
        git_hash        VARCHAR(64)  NOT NULL,
        git_author      NVARCHAR(256) NOT NULL,
        git_date        DATETIME2    NOT NULL,
        event           VARCHAR(16)  NOT NULL,
        error_text      NVARCHAR(MAX) NULL,
        created_at      DATETIME2    NOT NULL
    );
END TRY
BEGIN CATCH
    IF ERROR_NUMBER() <> 2714 THROW;
END CATCH;

-- Additive compatibility migration. Read-only plan/validate paths never run
-- this bootstrap against an existing history table; their load query treats a
-- missing column as legacy module drift instead.
IF COL_LENGTH('azdo_deploy_meta.history', 'live_definition_checksum') IS NULL
    ALTER TABLE azdo_deploy_meta.history
        ADD live_definition_checksum VARBINARY(32) NULL;

BEGIN TRY
    IF OBJECT_ID('azdo_deploy_meta.catalog_meta') IS NULL
    CREATE TABLE azdo_deploy_meta.catalog_meta (
        id             INT          NOT NULL CONSTRAINT PK_catalog_meta PRIMARY KEY,
        layout_digest  CHAR(64)     NOT NULL,
        object_count   INT          NOT NULL,
        captured_at    DATETIME2    NOT NULL
    );
END TRY
BEGIN CATCH
    IF ERROR_NUMBER() <> 2714 THROW;
END CATCH;

BEGIN TRY
    IF OBJECT_ID('azdo_deploy_meta.catalog_cache') IS NULL
    CREATE TABLE azdo_deploy_meta.catalog_cache (
        normalized_key  NVARCHAR(512) NOT NULL CONSTRAINT PK_catalog_cache PRIMARY KEY,
        schema_name     NVARCHAR(128) NOT NULL,
        kind            VARCHAR(32)   NOT NULL,
        object_name     NVARCHAR(256) NOT NULL,
        parent_name     NVARCHAR(256) NOT NULL,
        layout_digest   CHAR(64)      NOT NULL
    );
END TRY
BEGIN CATCH
    IF ERROR_NUMBER() <> 2714 THROW;
END CATCH;
