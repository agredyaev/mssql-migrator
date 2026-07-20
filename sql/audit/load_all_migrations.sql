SELECT h.normalized_key, h.checksum
FROM azdo_deploy_meta.history h
INNER JOIN (
    SELECT h2.normalized_key, MAX(h2.id) AS max_id
    FROM azdo_deploy_meta.history h2
    WHERE h2.kind = 'migration'
      AND h2.event = 'applied'
    GROUP BY h2.normalized_key
) latest ON h.normalized_key = latest.normalized_key AND h.id = latest.max_id;
