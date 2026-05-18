SELECT CASE
    WHEN compatibility_level >= 130 THEN 1
    ELSE 0
END AS openjson_supported
FROM sys.databases
WHERE database_id = DB_ID()
