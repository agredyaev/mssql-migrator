SELECT h.normalized_key, h.checksum
FROM azdo_deploy_meta.history h
INNER JOIN (
    SELECT normalized_key, MAX(id) AS max_id
    FROM azdo_deploy_meta.history
    WHERE kind = 'object'
      AND event IN ('applied', 'adopted')
      AND normalized_key IN ({{keys}})
    GROUP BY normalized_key
) latest ON h.normalized_key = latest.normalized_key AND h.id = latest.max_id;
