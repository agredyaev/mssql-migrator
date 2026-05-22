IF SCHEMA_ID('azdo_deploy_meta') IS NULL
    EXEC('CREATE SCHEMA azdo_deploy_meta');

IF OBJECT_ID('azdo_deploy_meta.history') IS NULL
CREATE TABLE azdo_deploy_meta.history (
    id              BIGINT IDENTITY(1,1) PRIMARY KEY,
    normalized_key  VARCHAR(512) NOT NULL,
    kind            VARCHAR(16)  NOT NULL,
    checksum        VARCHAR(64)  NOT NULL,
    git_hash        CHAR(40)     NOT NULL,
    git_author      VARCHAR(256) NOT NULL,
    git_date        DATETIME2    NOT NULL,
    event           VARCHAR(16)  NOT NULL,
    error_text      NVARCHAR(MAX) NULL,
    created_at      DATETIME2    NOT NULL
);

IF OBJECT_ID('azdo_deploy_meta.catalog_meta') IS NULL
CREATE TABLE azdo_deploy_meta.catalog_meta (
    id             INT          NOT NULL CONSTRAINT PK_catalog_meta PRIMARY KEY,
    layout_digest  CHAR(64)     NOT NULL,
    object_count   INT          NOT NULL,
    captured_at    DATETIME2    NOT NULL
);

IF OBJECT_ID('azdo_deploy_meta.catalog_cache') IS NULL
CREATE TABLE azdo_deploy_meta.catalog_cache (
    normalized_key  VARCHAR(512)  NOT NULL CONSTRAINT PK_catalog_cache PRIMARY KEY,
    schema_name     NVARCHAR(128) NOT NULL,
    kind            VARCHAR(32)   NOT NULL,
    object_name     NVARCHAR(256) NOT NULL,
    parent_name     NVARCHAR(256) NOT NULL,
    layout_digest   CHAR(64)      NOT NULL
);
