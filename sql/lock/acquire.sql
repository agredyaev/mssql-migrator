DECLARE @result INT;
EXEC @result = sp_getapplock
    @Resource = 'reporting_layer_migration',
    @LockMode = 'Exclusive',
    @LockOwner = 'Session',
    @LockTimeout = @p1;
SELECT @result;
