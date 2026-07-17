IF @@TRANCOUNT <> 1 THROW 50001, 'migration script body altered the executor transaction (BEGIN/COMMIT/ROLLBACK in script)', 1;
