INSERT INTO azdo_deploy_meta.history (normalized_key, kind, checksum, git_hash, git_author, git_date, event, error_text, created_at)
VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, SYSUTCDATETIME());
