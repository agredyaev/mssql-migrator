SELECT LOWER(s.name) AS schema_name
FROM sys.schemas s
WHERE LOWER(s.name) IN ({{schema_list}})
