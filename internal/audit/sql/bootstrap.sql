IF NOT EXISTS (SELECT 1 FROM sys.schemas WHERE name = 'azdo_deploy_meta') EXEC('CREATE SCHEMA azdo_deploy_meta');

IF OBJECT_ID('azdo_deploy_meta.history') IS NULL CREATE TABLE azdo_deploy_meta.history (
    id              BIGINT IDENTITY(1,1) PRIMARY KEY,
    normalized_key  VARCHAR(512) NOT NULL,
    kind            VARCHAR(16)  NOT NULL,   -- 'object' | 'migration'
    checksum        VARCHAR(64)  NOT NULL,
    git_hash        CHAR(40)     NOT NULL,
    git_author      VARCHAR(256) NOT NULL,
    git_date        DATETIME2    NOT NULL,
    event           VARCHAR(16)  NOT NULL,   -- 'applied' | 'adopted' | 'failed'
    error_text      NVARCHAR(MAX) NULL,
    created_at      DATETIME2    NOT NULL
);

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_history_key_kind')
    CREATE INDEX IX_history_key_kind ON azdo_deploy_meta.history(normalized_key, kind, id DESC);
