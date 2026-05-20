SELECT normalized_key
FROM azdo_deploy_meta.history
WHERE kind = 'migration'
  AND event = 'applied'
