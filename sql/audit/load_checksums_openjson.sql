WITH checksum_keys AS (
    SELECT DISTINCT CONVERT(nvarchar(512), [value]) COLLATE DATABASE_DEFAULT AS normalized_key
    FROM OPENJSON(@p1)
)
SELECT h.normalized_key, h.checksum
FROM azdo_deploy_meta.history h
INNER JOIN (
    SELECT h2.normalized_key, MAX(h2.id) AS max_id
    FROM azdo_deploy_meta.history h2
    JOIN checksum_keys k ON k.normalized_key = h2.normalized_key COLLATE DATABASE_DEFAULT
    WHERE h2.kind = 'object'
      AND h2.event IN ('applied', 'adopted')
    GROUP BY h2.normalized_key
) latest ON h.normalized_key COLLATE DATABASE_DEFAULT = latest.normalized_key AND h.id = latest.max_id;
