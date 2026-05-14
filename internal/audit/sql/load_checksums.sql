SELECT normalized_key, checksum
FROM __migrator.object_state
WHERE normalized_key IN ({{keys}})
