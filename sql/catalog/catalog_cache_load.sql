SELECT c.normalized_key,
       c.schema_name,
       c.kind,
       c.object_name,
       c.parent_name
FROM azdo_deploy_meta.catalog_cache c
INNER JOIN azdo_deploy_meta.catalog_meta m ON m.id = 1
WHERE m.layout_digest = @p1
  AND m.object_count = @p2
  AND c.layout_digest = m.layout_digest;
