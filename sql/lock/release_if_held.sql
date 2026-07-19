IF APPLOCK_MODE('public', 'reporting_layer_migration', 'Session') <> 'NoLock' EXEC sp_releaseapplock @Resource = 'reporting_layer_migration', @LockOwner = 'Session';
