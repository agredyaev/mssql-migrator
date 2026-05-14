-- Create test database for rmig integration tests
-- Usage: docker compose exec -T mssql /opt/mssql-tools18/bin/sqlcmd \
--        -S localhost -U sa -P 'yourStrong(!)Password' -C -i scripts/sql/create-test-db.sql

IF DB_ID('rmig_test') IS NULL
BEGIN
    CREATE DATABASE rmig_test
    PRINT 'Created rmig_test database'
END
ELSE
    PRINT 'rmig_test database already exists'
GO
