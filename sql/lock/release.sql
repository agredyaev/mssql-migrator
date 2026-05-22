DECLARE @result INT;
EXEC @result = sp_releaseapplock
    @Resource = 'reporting_layer_migration',
    @LockOwner = 'Session';
SELECT @result;
