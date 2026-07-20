-- Best-effort DDL trigger that stamps a monotonic version onto every managed
-- object whenever it changes, so the plan phase can fingerprint only objects
-- touched since their last apply instead of the whole catalog. The object_ddl
-- table, ddl_seq sequence, and history.applied_ddl_version column it depends on
-- are created in bootstrap_tables.sql (permission-free). If the deploy principal
-- cannot create a database DDL trigger, this is skipped and the read path
-- detects the absent/disabled trigger and full-fingerprints as before.

-- The trigger body calls XML data-type methods on EVENTDATA(), which require
-- QUOTED_IDENTIFIER ON at creation time (triggers persist their SET options).
SET QUOTED_IDENTIFIER ON;
SET ANSI_NULLS ON;

-- CREATE TRIGGER must be the first statement in its batch, so it is wrapped in
-- EXEC. Each managed object (including standalone indexes, which are tracked
-- separately from their table) is keyed by its own EVENTDATA ObjectName.
BEGIN TRY
    IF NOT EXISTS (SELECT 1 FROM sys.triggers WHERE parent_class = 0 AND name = 'azdo_deploy_meta_ddl_watch')
    EXEC('
CREATE TRIGGER azdo_deploy_meta_ddl_watch ON DATABASE
FOR CREATE_TABLE, ALTER_TABLE, DROP_TABLE,
    CREATE_INDEX, ALTER_INDEX, DROP_INDEX,
    CREATE_VIEW, ALTER_VIEW, DROP_VIEW,
    CREATE_PROCEDURE, ALTER_PROCEDURE, DROP_PROCEDURE,
    CREATE_FUNCTION, ALTER_FUNCTION, DROP_FUNCTION,
    CREATE_TRIGGER, ALTER_TRIGGER, DROP_TRIGGER,
    CREATE_SYNONYM, DROP_SYNONYM,
    CREATE_SEQUENCE, ALTER_SEQUENCE, DROP_SEQUENCE,
    CREATE_TYPE, DROP_TYPE
AS
BEGIN
    SET NOCOUNT ON;
    DECLARE @d XML = EVENTDATA();
    DECLARE @schema NVARCHAR(128) = @d.value(''(/EVENT_INSTANCE/SchemaName)[1]'', ''nvarchar(128)'');
    DECLARE @obj NVARCHAR(256) = @d.value(''(/EVENT_INSTANCE/ObjectName)[1]'', ''nvarchar(256)'');
    IF @schema IS NULL OR @obj IS NULL OR LOWER(@schema) = ''azdo_deploy_meta'' RETURN;
    -- Store lowercased to match the migrator normalized keys (schema/kind/name).
    SET @schema = LOWER(@schema);
    SET @obj = LOWER(@obj);
    DECLARE @v BIGINT = NEXT VALUE FOR azdo_deploy_meta.ddl_seq;
    UPDATE azdo_deploy_meta.object_ddl SET ddl_version = @v, updated_at = SYSUTCDATETIME()
        WHERE schema_name = @schema AND object_name = @obj;
    IF @@ROWCOUNT = 0
        INSERT INTO azdo_deploy_meta.object_ddl (schema_name, object_name, ddl_version, updated_at)
        VALUES (@schema, @obj, @v, SYSUTCDATETIME());
END');
END TRY
BEGIN CATCH
    -- No permission to create a database DDL trigger (or a race): leave it
    -- absent; the read path detects this and full-fingerprints as before.
    IF ERROR_NUMBER() NOT IN (1785, 2714, 3701) AND ERROR_NUMBER() < 15000 THROW;
END CATCH;
