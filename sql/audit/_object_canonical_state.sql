    OUTER APPLY (
        SELECT CASE WHEN NULLIF(state.canonical_state, N'') IS NULL THEN NULL
            ELSE HASHBYTES('SHA2_256', CONCAT(rows.object_kind, N':', state.canonical_state))
            END AS definition_checksum
        FROM (SELECT CASE rows.object_kind
            WHEN 'views' THEN (
                SELECT OBJECT_DEFINITION(o.object_id)
                FROM sys.objects AS o
                INNER JOIN sys.schemas AS s ON s.schema_id = o.schema_id
                WHERE LOWER(s.name) = rows.schema_name COLLATE DATABASE_DEFAULT
                  AND LOWER(o.name) = rows.object_name COLLATE DATABASE_DEFAULT
                  AND o.type = 'V'
            )
            WHEN 'functions' THEN (
                SELECT OBJECT_DEFINITION(o.object_id)
                FROM sys.objects AS o
                INNER JOIN sys.schemas AS s ON s.schema_id = o.schema_id
                WHERE LOWER(s.name) = rows.schema_name COLLATE DATABASE_DEFAULT
                  AND LOWER(o.name) = rows.object_name COLLATE DATABASE_DEFAULT
                  AND o.type IN ('FN', 'IF', 'TF', 'FS', 'FT')
            )
            WHEN 'procedures' THEN (
                SELECT OBJECT_DEFINITION(o.object_id)
                FROM sys.objects AS o
                INNER JOIN sys.schemas AS s ON s.schema_id = o.schema_id
                WHERE LOWER(s.name) = rows.schema_name COLLATE DATABASE_DEFAULT
                  AND LOWER(o.name) = rows.object_name COLLATE DATABASE_DEFAULT
                  AND o.type IN ('P', 'PC')
            )
            WHEN 'triggers' THEN (
                SELECT OBJECT_DEFINITION(o.object_id)
                FROM sys.objects AS o
                INNER JOIN sys.schemas AS s ON s.schema_id = o.schema_id
                WHERE LOWER(s.name) = rows.schema_name COLLATE DATABASE_DEFAULT
                  AND LOWER(o.name) = rows.object_name COLLATE DATABASE_DEFAULT
                  AND o.type = 'TR'
            )
            WHEN 'synonyms' THEN (
                SELECT sy.base_object_name
                FROM sys.synonyms AS sy
                INNER JOIN sys.schemas AS s ON s.schema_id = sy.schema_id
                WHERE LOWER(s.name) = rows.schema_name COLLATE DATABASE_DEFAULT
                  AND LOWER(sy.name) = rows.object_name COLLATE DATABASE_DEFAULT
            )
            WHEN 'sequences' THEN (
                SELECT seq.system_type_id, seq.user_type_id, seq.precision, seq.scale,
                    CONVERT(nvarchar(128), seq.start_value) AS start_value,
                    CONVERT(nvarchar(128), seq.increment) AS increment_value,
                    CONVERT(nvarchar(128), seq.minimum_value) AS minimum_value,
                    CONVERT(nvarchar(128), seq.maximum_value) AS maximum_value,
                    seq.is_cycling, seq.is_cached, seq.cache_size
                FROM sys.sequences AS seq
                INNER JOIN sys.schemas AS s ON s.schema_id = seq.schema_id
                WHERE LOWER(s.name) = rows.schema_name COLLATE DATABASE_DEFAULT
                  AND LOWER(seq.name) = rows.object_name COLLATE DATABASE_DEFAULT
                FOR JSON PATH, WITHOUT_ARRAY_WRAPPER, INCLUDE_NULL_VALUES
            )
            WHEN 'indexes' THEN CASE WHEN EXISTS (
                SELECT 1
                FROM sys.indexes AS existing_index
                INNER JOIN sys.objects AS existing_parent
                    ON existing_parent.object_id = existing_index.object_id
                INNER JOIN sys.schemas AS existing_schema
                    ON existing_schema.schema_id = existing_parent.schema_id
                WHERE LOWER(existing_schema.name) = rows.schema_name COLLATE DATABASE_DEFAULT
                  AND LOWER(existing_index.name) = rows.object_name COLLATE DATABASE_DEFAULT
                  AND existing_index.is_hypothetical = 0
                  AND existing_parent.type IN ('U', 'V')
            ) THEN (
                SELECT LOWER(ps.name) AS parent_schema, LOWER(parent.name) AS parent_name,
                    i.type, i.is_unique, LOWER(ds.name) AS data_space,
                    i.ignore_dup_key, i.is_primary_key, i.is_unique_constraint,
                    i.fill_factor, i.is_padded, i.is_disabled,
                    i.allow_row_locks, i.allow_page_locks, i.has_filter, i.filter_definition,
                    st.no_recompute, st.is_incremental, hi.bucket_count,
                    xi.secondary_type,
                    JSON_QUERY((
                        SELECT ic.index_column_id, ic.key_ordinal, ic.partition_ordinal,
                            ic.is_descending_key, ic.is_included_column, LOWER(c.name) AS column_name
                        FROM sys.index_columns AS ic
                        INNER JOIN sys.columns AS c
                            ON c.object_id = ic.object_id AND c.column_id = ic.column_id
                        WHERE ic.object_id = i.object_id AND ic.index_id = i.index_id
                        ORDER BY ic.index_column_id
                        FOR JSON PATH, INCLUDE_NULL_VALUES
                    )) AS columns,
                    JSON_QUERY((
                        SELECT p.partition_number, p.data_compression
                        FROM sys.partitions AS p
                        WHERE p.object_id = i.object_id AND p.index_id = i.index_id
                        ORDER BY p.partition_number
                        FOR JSON PATH, INCLUDE_NULL_VALUES
                    )) AS partitions
                FROM sys.indexes AS i
                INNER JOIN sys.objects AS parent ON parent.object_id = i.object_id
                INNER JOIN sys.schemas AS ps ON ps.schema_id = parent.schema_id
                LEFT JOIN sys.data_spaces AS ds ON ds.data_space_id = i.data_space_id
                LEFT JOIN sys.stats AS st ON st.object_id = i.object_id AND st.stats_id = i.index_id
                LEFT JOIN sys.hash_indexes AS hi ON hi.object_id = i.object_id AND hi.index_id = i.index_id
                LEFT JOIN sys.xml_indexes AS xi ON xi.object_id = i.object_id AND xi.index_id = i.index_id
                WHERE LOWER(ps.name) = rows.schema_name COLLATE DATABASE_DEFAULT
                  AND LOWER(i.name) = rows.object_name COLLATE DATABASE_DEFAULT
                  AND i.is_hypothetical = 0
                  AND parent.type IN ('U', 'V')
                ORDER BY LOWER(parent.name)
                FOR JSON PATH, INCLUDE_NULL_VALUES
            ) END
            WHEN 'types' THEN (
                SELECT ty.is_table_type, ty.max_length, ty.precision, ty.scale,
                    ty.collation_name, ty.is_nullable,
                    LOWER(base_schema.name) AS base_type_schema, LOWER(base_type.name) AS base_type_name,
                    OBJECT_DEFINITION(ty.default_object_id) AS default_definition,
                    OBJECT_DEFINITION(ty.rule_object_id) AS rule_definition,
                    JSON_QUERY((
                        SELECT c.column_id, LOWER(c.name) AS column_name,
                            LOWER(type_schema.name) AS type_schema, LOWER(column_type.name) AS type_name,
                            c.max_length, c.precision, c.scale, c.collation_name, c.is_nullable,
                            c.is_ansi_padded, c.is_rowguidcol, c.is_identity,
                            CONVERT(nvarchar(128), ident.seed_value) AS identity_seed,
                            CONVERT(nvarchar(128), ident.increment_value) AS identity_increment,
                            c.is_computed, computed.definition AS computed_definition,
                            computed.is_persisted, defaults.name AS default_name,
                            defaults.definition AS default_definition
                        FROM sys.columns AS c
                        INNER JOIN sys.types AS column_type ON column_type.user_type_id = c.user_type_id
                        INNER JOIN sys.schemas AS type_schema ON type_schema.schema_id = column_type.schema_id
                        LEFT JOIN sys.identity_columns AS ident
                            ON ident.object_id = c.object_id AND ident.column_id = c.column_id
                        LEFT JOIN sys.computed_columns AS computed
                            ON computed.object_id = c.object_id AND computed.column_id = c.column_id
                        LEFT JOIN sys.default_constraints AS defaults ON defaults.object_id = c.default_object_id
                        WHERE c.object_id = table_type.type_table_object_id
                        ORDER BY c.column_id
                        FOR JSON PATH, INCLUDE_NULL_VALUES
                    )) AS columns,
                    JSON_QUERY((
                        SELECT checks.name, checks.parent_column_id, checks.definition,
                            checks.is_disabled, checks.is_not_trusted, checks.is_not_for_replication
                        FROM sys.check_constraints AS checks
                        WHERE checks.parent_object_id = table_type.type_table_object_id
                        ORDER BY checks.name
                        FOR JSON PATH, INCLUDE_NULL_VALUES
                    )) AS checks,
                    JSON_QUERY((
                        SELECT idx.name, idx.type, idx.is_unique, idx.is_primary_key,
                            idx.is_unique_constraint, idx.ignore_dup_key,
                            JSON_QUERY((
                                SELECT ic.index_column_id, ic.key_ordinal, ic.is_descending_key,
                                    ic.is_included_column, LOWER(c.name) AS column_name
                                FROM sys.index_columns AS ic
                                INNER JOIN sys.columns AS c
                                    ON c.object_id = ic.object_id AND c.column_id = ic.column_id
                                WHERE ic.object_id = idx.object_id AND ic.index_id = idx.index_id
                                ORDER BY ic.index_column_id
                                FOR JSON PATH, INCLUDE_NULL_VALUES
                            )) AS columns
                        FROM sys.indexes AS idx
                        WHERE idx.object_id = table_type.type_table_object_id
                          AND idx.index_id > 0 AND idx.is_hypothetical = 0
                        ORDER BY idx.index_id
                        FOR JSON PATH, INCLUDE_NULL_VALUES
                    )) AS indexes
                FROM sys.types AS ty
                INNER JOIN sys.schemas AS s ON s.schema_id = ty.schema_id
                LEFT JOIN sys.types AS base_type
                    ON base_type.user_type_id = ty.system_type_id
                   AND base_type.user_type_id = base_type.system_type_id
                LEFT JOIN sys.schemas AS base_schema ON base_schema.schema_id = base_type.schema_id
                LEFT JOIN sys.table_types AS table_type ON table_type.user_type_id = ty.user_type_id
                WHERE ty.is_user_defined = 1
                  AND LOWER(s.name) = rows.schema_name COLLATE DATABASE_DEFAULT
                  AND LOWER(ty.name) = rows.object_name COLLATE DATABASE_DEFAULT
                FOR JSON PATH, WITHOUT_ARRAY_WRAPPER, INCLUDE_NULL_VALUES
            )
            WHEN 'tables' THEN (
                SELECT table_state.temporal_type, table_state.is_memory_optimized,
                    table_state.durability, table_state.is_filetable,
                    LOWER(history_schema.name) AS history_schema, LOWER(history_table.name) AS history_table,
                    LOWER(data_space.name) AS data_space,
                    JSON_QUERY((
                        SELECT c.column_id, LOWER(c.name) AS column_name,
                            LOWER(type_schema.name) AS type_schema, LOWER(column_type.name) AS type_name,
                            c.max_length, c.precision, c.scale, c.collation_name, c.is_nullable,
                            c.is_ansi_padded, c.is_rowguidcol, c.is_identity,
                            CONVERT(nvarchar(128), ident.seed_value) AS identity_seed,
                            CONVERT(nvarchar(128), ident.increment_value) AS identity_increment,
                            c.is_computed, computed.definition AS computed_definition,
                            computed.is_persisted, c.is_filestream, c.is_xml_document,
                            LOWER(xml_schema.name) AS xml_schema, LOWER(xml_collection.name) AS xml_collection,
                            defaults.name AS default_name, defaults.definition AS default_definition,
                            c.is_sparse, c.is_column_set, c.generated_always_type,
                            c.encryption_type, c.encryption_algorithm_name,
                            LOWER(encryption_key.name) AS column_encryption_key,
                            c.is_hidden, masked.masking_function
                        FROM sys.columns AS c
                        INNER JOIN sys.types AS column_type ON column_type.user_type_id = c.user_type_id
                        INNER JOIN sys.schemas AS type_schema ON type_schema.schema_id = column_type.schema_id
                        LEFT JOIN sys.identity_columns AS ident
                            ON ident.object_id = c.object_id AND ident.column_id = c.column_id
                        LEFT JOIN sys.computed_columns AS computed
                            ON computed.object_id = c.object_id AND computed.column_id = c.column_id
                        LEFT JOIN sys.default_constraints AS defaults ON defaults.object_id = c.default_object_id
                        LEFT JOIN sys.xml_schema_collections AS xml_collection
                            ON xml_collection.xml_collection_id = c.xml_collection_id
                        LEFT JOIN sys.schemas AS xml_schema ON xml_schema.schema_id = xml_collection.schema_id
                        LEFT JOIN sys.column_encryption_keys AS encryption_key
                            ON encryption_key.column_encryption_key_id = c.column_encryption_key_id
                        LEFT JOIN sys.masked_columns AS masked
                            ON masked.object_id = c.object_id AND masked.column_id = c.column_id
                        WHERE c.object_id = table_state.object_id
                        ORDER BY c.column_id
                        FOR JSON PATH, INCLUDE_NULL_VALUES
                    )) AS columns,
                    JSON_QUERY((
                        SELECT checks.name, checks.parent_column_id, checks.definition,
                            checks.is_disabled, checks.is_not_trusted, checks.is_not_for_replication
                        FROM sys.check_constraints AS checks
                        WHERE checks.parent_object_id = table_state.object_id
                        ORDER BY checks.name
                        FOR JSON PATH, INCLUDE_NULL_VALUES
                    )) AS checks,
                    JSON_QUERY((
                        SELECT fk.name, LOWER(ref_schema.name) AS referenced_schema,
                            LOWER(ref_table.name) AS referenced_table,
                            fk.delete_referential_action, fk.update_referential_action,
                            fk.is_disabled, fk.is_not_trusted, fk.is_not_for_replication,
                            JSON_QUERY((
                                SELECT fkc.constraint_column_id,
                                    LOWER(parent_column.name) AS parent_column,
                                    LOWER(ref_column.name) AS referenced_column
                                FROM sys.foreign_key_columns AS fkc
                                INNER JOIN sys.columns AS parent_column
                                    ON parent_column.object_id = fkc.parent_object_id
                                   AND parent_column.column_id = fkc.parent_column_id
                                INNER JOIN sys.columns AS ref_column
                                    ON ref_column.object_id = fkc.referenced_object_id
                                   AND ref_column.column_id = fkc.referenced_column_id
                                WHERE fkc.constraint_object_id = fk.object_id
                                ORDER BY fkc.constraint_column_id
                                FOR JSON PATH, INCLUDE_NULL_VALUES
                            )) AS columns
                        FROM sys.foreign_keys AS fk
                        INNER JOIN sys.objects AS ref_table ON ref_table.object_id = fk.referenced_object_id
                        INNER JOIN sys.schemas AS ref_schema ON ref_schema.schema_id = ref_table.schema_id
                        WHERE fk.parent_object_id = table_state.object_id
                        ORDER BY fk.name
                        FOR JSON PATH, INCLUDE_NULL_VALUES
                    )) AS foreign_keys,
                    JSON_QUERY((
                        SELECT keys.name, keys.type AS constraint_type,
                            idx.type AS index_type, idx.is_unique,
                            idx.ignore_dup_key, idx.fill_factor, idx.is_padded,
                            JSON_QUERY((
                                SELECT ic.index_column_id, ic.key_ordinal, ic.is_descending_key,
                                    LOWER(c.name) AS column_name
                                FROM sys.index_columns AS ic
                                INNER JOIN sys.columns AS c
                                    ON c.object_id = ic.object_id AND c.column_id = ic.column_id
                                WHERE ic.object_id = idx.object_id AND ic.index_id = idx.index_id
                                  AND ic.is_included_column = 0
                                ORDER BY ic.key_ordinal
                                FOR JSON PATH, INCLUDE_NULL_VALUES
                            )) AS columns
                        FROM sys.key_constraints AS keys
                        INNER JOIN sys.indexes AS idx
                            ON idx.object_id = keys.parent_object_id
                           AND idx.index_id = keys.unique_index_id
                        WHERE keys.parent_object_id = table_state.object_id
                        ORDER BY keys.name
                        FOR JSON PATH, INCLUDE_NULL_VALUES
                    )) AS keys
                FROM sys.tables AS table_state
                INNER JOIN sys.schemas AS s ON s.schema_id = table_state.schema_id
                LEFT JOIN sys.tables AS history_table ON history_table.object_id = table_state.history_table_id
                LEFT JOIN sys.schemas AS history_schema ON history_schema.schema_id = history_table.schema_id
                LEFT JOIN sys.indexes AS heap_or_clustered
                    ON heap_or_clustered.object_id = table_state.object_id
                   AND heap_or_clustered.index_id IN (0, 1)
                LEFT JOIN sys.data_spaces AS data_space
                    ON data_space.data_space_id = heap_or_clustered.data_space_id
                WHERE LOWER(s.name) = rows.schema_name COLLATE DATABASE_DEFAULT
                  AND LOWER(table_state.name) = rows.object_name COLLATE DATABASE_DEFAULT
                FOR JSON PATH, WITHOUT_ARRAY_WRAPPER, INCLUDE_NULL_VALUES
            ) END AS canonical_state
            -- Incremental drift: filter the row out before the expensive
            -- per-kind subqueries are projected, so non-suspect objects skip the
            -- fingerprint entirely (a CASE guard is not enough — the optimizer
            -- may still evaluate the subqueries). The insert path sets
            -- is_suspect = 1 for every row, so it always fingerprints.
            FROM (VALUES (1)) AS suspect_guard(x)
            WHERE rows.is_suspect = 1) AS state
        WHERE rows.kind = 'object'
    ) AS live;
