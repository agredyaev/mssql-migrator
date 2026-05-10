package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/planner"
	"reporting-db-migrations/internal/validate"
)

type scopeWriter struct {
	writer metadataWriter
	conn   *sql.Conn
}

func (s scopeWriter) Migration(ctx context.Context, plan contracts.MigrationPlan) (map[string]int64, error) {
	if s.writer.runID == "" {
		return nil, fmt.Errorf("persist migration scope: missing run id")
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin metadata scope transaction: %w", err)
	}
	for _, schema := range plan.Schemas {
		exists := schema.Exists
		record := metadata.ItemRecord{
			RunID:                s.writer.runID,
			ItemType:             metadata.ItemTypeSchema,
			SchemaName:           schema.SchemaName,
			NormalizedSchemaName: strings.ToLower(schema.SchemaName),
			NormalizedKey:        strings.ToLower(schema.SchemaName),
			ExistsInDatabase:     boolPtr(exists),
			Action:               schema.Action,
		}
		if schema.Action == contracts.SchemaActionExists {
			record.Success = boolPtr(true)
		}
		if _, err := metadata.InsertItem(ctx, tx, record); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	itemIDs := map[string]int64{}
	for _, object := range plan.Objects {
		exists := object.Exists
		record := metadata.ItemRecord{
			RunID:                s.writer.runID,
			ItemType:             metadata.ItemTypeObject,
			ObjectPath:           object.ObjectPath,
			SchemaName:           object.SchemaName,
			NormalizedSchemaName: strings.ToLower(object.SchemaName),
			Kind:                 object.Kind,
			ObjectName:           object.ObjectName,
			NormalizedObjectName: strings.ToLower(object.ObjectName),
			ParentName:           object.ParentName,
			NormalizedParentName: strings.ToLower(object.ParentName),
			NormalizedKey:        object.NormalizedKey,
			Checksum:             object.Checksum,
			ExistsInDatabase:     boolPtr(exists),
			MetadataMatch:        object.MetadataMatch,
			Action:               object.PlannedAction,
		}
		switch object.PlannedAction {
		case contracts.ActionSkipUnchanged, contracts.ActionAdoptExisting:
			record.Success = boolPtr(true)
		case contracts.ActionFail, contracts.ActionReprocessChangedBlocked:
			record.Success = boolPtr(false)
		}
		id, err := metadata.InsertItem(ctx, tx, record)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		itemIDs[object.NormalizedKey] = id
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit metadata scope transaction: %w", err)
	}
	return itemIDs, nil
}

func (s scopeWriter) Validation(ctx context.Context, layout parser.Layout, catalog validate.CatalogState, successfulByKey map[string]string) (map[string]int64, error) {
	if s.writer.runID == "" {
		return nil, fmt.Errorf("persist validation scope: missing run id")
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin validation scope transaction: %w", err)
	}
	for _, schema := range layout.Schemas {
		_, exists := catalog.Schemas[schema.NormalizedName]
		action := contracts.SchemaActionExists
		record := metadata.ItemRecord{
			RunID:                s.writer.runID,
			ItemType:             metadata.ItemTypeSchema,
			SchemaName:           schema.Name,
			NormalizedSchemaName: schema.NormalizedName,
			NormalizedKey:        schema.NormalizedName,
			ExistsInDatabase:     boolPtr(exists),
			Action:               action,
			Success:              boolPtr(exists),
		}
		if !exists {
			record.Action = contracts.ActionFail
			record.ErrorMessage = fmt.Sprintf("missing schema: %s", schema.Name)
		}
		if _, err := metadata.InsertItem(ctx, tx, record); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	itemIDs := map[string]int64{}
	for _, object := range layout.Objects {
		_, exists := catalog.Objects[object.NormalizedKey]
		var metadataMatch *bool
		if checksum, ok := successfulByKey[object.NormalizedKey]; ok {
			metadataMatch = boolPtr(checksum == object.Checksum)
		}
		record := metadata.ItemRecord{
			RunID:                s.writer.runID,
			ItemType:             metadata.ItemTypeObject,
			ObjectPath:           object.Path,
			SchemaName:           object.SchemaName,
			NormalizedSchemaName: object.NormalizedSchemaName,
			Kind:                 object.Kind,
			ObjectName:           object.ObjectName,
			NormalizedObjectName: object.NormalizedObjectName,
			ParentName:           object.ParentName,
			NormalizedParentName: object.NormalizedParentName,
			NormalizedKey:        object.NormalizedKey,
			Checksum:             object.Checksum,
			ExistsInDatabase:     boolPtr(exists),
			MetadataMatch:        metadataMatch,
			Action:               validationObjectAction(object.Kind),
		}
		if !exists {
			record.Success = boolPtr(false)
			record.ErrorMessage = fmt.Sprintf("missing managed object: %s", object.Path)
		}
		id, err := metadata.InsertItem(ctx, tx, record)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		itemIDs[object.NormalizedKey] = id
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit validation scope transaction: %w", err)
	}
	return itemIDs, nil
}

func (s scopeWriter) Repair(ctx context.Context, object parser.Object, planned contracts.PlannedObject, catalog planner.CatalogState, currentChecksum string) (*int64, error) {
	if s.writer.runID == "" {
		return nil, fmt.Errorf("persist repair scope: missing run id")
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin repair scope transaction: %w", err)
	}
	_, schemaExists := catalog.Schemas[object.NormalizedSchemaName]
	if _, err := metadata.InsertItem(ctx, tx, metadata.ItemRecord{
		RunID:                s.writer.runID,
		ItemType:             metadata.ItemTypeSchema,
		SchemaName:           object.SchemaName,
		NormalizedSchemaName: object.NormalizedSchemaName,
		NormalizedKey:        object.NormalizedSchemaName,
		ExistsInDatabase:     boolPtr(schemaExists),
		Action:               contracts.SchemaActionExists,
		Success:              boolPtr(schemaExists),
	}); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	metadataMatch := currentChecksum == object.Checksum
	itemID, err := metadata.InsertItem(ctx, tx, metadata.ItemRecord{
		RunID:                s.writer.runID,
		ItemType:             metadata.ItemTypeObject,
		ObjectPath:           object.Path,
		SchemaName:           object.SchemaName,
		NormalizedSchemaName: object.NormalizedSchemaName,
		Kind:                 object.Kind,
		ObjectName:           object.ObjectName,
		NormalizedObjectName: object.NormalizedObjectName,
		ParentName:           object.ParentName,
		NormalizedParentName: object.NormalizedParentName,
		NormalizedKey:        object.NormalizedKey,
		Checksum:             object.Checksum,
		ExistsInDatabase:     boolPtr(true),
		MetadataMatch:        boolPtr(metadataMatch),
		Action:               planned.PlannedAction,
	})
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit repair scope transaction: %w", err)
	}
	return &itemID, nil
}
