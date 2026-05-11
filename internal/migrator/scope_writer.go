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

func (s scopeWriter) requireConn(operation string) error {
	if s.conn == nil {
		return fmt.Errorf("%s: missing metadata connection", operation)
	}
	return nil
}

func rollbackWithContext(tx *sql.Tx, err error, operation string) error {
	if tx == nil {
		return err
	}
	if rollbackErr := tx.Rollback(); rollbackErr != nil {
		return fmt.Errorf("%s: %w; rollback failed: %v", operation, err, rollbackErr)
	}
	return err
}

const scopeInsertChunkSize = 100

func (s scopeWriter) Migration(ctx context.Context, plan contracts.MigrationPlan) (map[string]int64, error) {
	if s.writer.runID == "" {
		return nil, fmt.Errorf("persist migration scope: missing run id")
	}
	if err := s.requireConn("persist migration scope"); err != nil {
		return nil, err
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin metadata scope transaction: %w", err)
	}
	schemaRecords := make([]metadata.ItemRecord, 0, len(plan.Schemas))
	for _, schema := range plan.Schemas {
		exists := schema.Exists
		record := metadata.ItemRecord{
			RunID:            s.writer.runID,
			SchemaName:       schema.SchemaName,
			Kind:             "schema",
			NormalizedKey:    strings.ToLower(schema.SchemaName),
			ExistsInDatabase: boolPtr(exists),
			Action:           schema.Action,
		}
		if schema.Action == contracts.SchemaActionExists {
			record.Success = boolPtr(true)
		}
		schemaRecords = append(schemaRecords, record)
	}
	if err := insertItemRecordsInChunks(ctx, tx, schemaRecords); err != nil {
		return nil, rollbackWithContext(tx, err, "persist migration scope")
	}
	objectRecords := make([]metadata.ItemRecord, 0, len(plan.Objects))
	for _, object := range plan.Objects {
		exists := object.Exists
		record := metadata.ItemRecord{
			RunID:            s.writer.runID,
			ObjectPath:       object.ObjectPath,
			SchemaName:       object.SchemaName,
			Kind:             object.Kind,
			ObjectName:       object.ObjectName,
			ParentName:       object.ParentName,
			NormalizedKey:    object.NormalizedKey,
			Checksum:         object.Checksum,
			ExistsInDatabase: boolPtr(exists),
			MetadataMatch:    object.MetadataMatch,
			Action:           object.PlannedAction,
		}
		switch object.PlannedAction {
		case contracts.ActionSkipUnchanged, contracts.ActionAdoptExisting:
			record.Success = boolPtr(true)
		case contracts.ActionFail, contracts.ActionReprocessChangedBlocked:
			record.Success = boolPtr(false)
		}
		objectRecords = append(objectRecords, record)
	}
	if err := insertItemRecordsInChunks(ctx, tx, objectRecords); err != nil {
		return nil, rollbackWithContext(tx, err, "persist migration scope")
	}
	itemIDs, err := s.writer.loadItemIDs(ctx, tx, false)
	if err != nil {
		return nil, rollbackWithContext(tx, err, "persist migration scope")
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
	if err := s.requireConn("persist validation scope"); err != nil {
		return nil, err
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin validation scope transaction: %w", err)
	}
	schemaRecords := make([]metadata.ItemRecord, 0, len(layout.Schemas))
	for _, schema := range layout.Schemas {
		_, exists := catalog.Schemas[schema.NormalizedName]
		action := contracts.SchemaActionExists
		record := metadata.ItemRecord{
			RunID:            s.writer.runID,
			SchemaName:       schema.Name,
			Kind:             "schema",
			NormalizedKey:    schema.NormalizedName,
			ExistsInDatabase: boolPtr(exists),
			Action:           action,
			Success:          boolPtr(exists),
		}
		if !exists {
			record.Action = contracts.ActionFail
			record.ErrorMessage = fmt.Sprintf("missing schema: %s", schema.Name)
		}
		schemaRecords = append(schemaRecords, record)
	}
	if err := insertItemRecordsInChunks(ctx, tx, schemaRecords); err != nil {
		return nil, rollbackWithContext(tx, err, "persist validation scope")
	}
	objectRecords := make([]metadata.ItemRecord, 0, len(layout.Objects))
	for _, object := range layout.Objects {
		_, exists := catalog.Objects[object.NormalizedKey]
		var metadataMatch *bool
		if checksum, ok := successfulByKey[object.NormalizedKey]; ok {
			metadataMatch = boolPtr(checksum == object.Checksum)
		}
		record := metadata.ItemRecord{
			RunID:            s.writer.runID,
			ObjectPath:       object.Path,
			SchemaName:       object.SchemaName,
			Kind:             object.Kind,
			ObjectName:       object.ObjectName,
			ParentName:       object.ParentName,
			NormalizedKey:    object.NormalizedKey,
			Checksum:         object.Checksum,
			ExistsInDatabase: boolPtr(exists),
			MetadataMatch:    metadataMatch,
			Action:           validationObjectAction(object.Kind),
		}
		if !exists {
			record.Success = boolPtr(false)
			record.ErrorMessage = fmt.Sprintf("missing managed object: %s", object.Path)
		}
		objectRecords = append(objectRecords, record)
	}
	if err := insertItemRecordsInChunks(ctx, tx, objectRecords); err != nil {
		return nil, rollbackWithContext(tx, err, "persist validation scope")
	}
	itemIDs, err := s.writer.loadItemIDs(ctx, tx, false)
	if err != nil {
		return nil, rollbackWithContext(tx, err, "persist validation scope")
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
	if err := s.requireConn("persist repair scope"); err != nil {
		return nil, err
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin repair scope transaction: %w", err)
	}
	_, schemaExists := catalog.Schemas[object.NormalizedSchemaName]
	if _, err := metadata.InsertItem(ctx, tx, metadata.ItemRecord{
		RunID:            s.writer.runID,
		SchemaName:       object.SchemaName,
		Kind:             "schema",
		NormalizedKey:    object.NormalizedSchemaName,
		ExistsInDatabase: boolPtr(schemaExists),
		Action:           contracts.SchemaActionExists,
		Success:          boolPtr(schemaExists),
	}); err != nil {
		return nil, rollbackWithContext(tx, err, "persist repair scope")
	}
	metadataMatch := currentChecksum == object.Checksum
	itemID, err := metadata.InsertItem(ctx, tx, metadata.ItemRecord{
		RunID:            s.writer.runID,
		ObjectPath:       object.Path,
		SchemaName:       object.SchemaName,
		Kind:             object.Kind,
		ObjectName:       object.ObjectName,
		ParentName:       object.ParentName,
		NormalizedKey:    object.NormalizedKey,
		Checksum:         object.Checksum,
		ExistsInDatabase: boolPtr(true),
		MetadataMatch:    boolPtr(metadataMatch),
		Action:           planned.PlannedAction,
	})
	if err != nil {
		return nil, rollbackWithContext(tx, err, "persist repair scope")
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit repair scope transaction: %w", err)
	}
	return &itemID, nil
}

func insertItemRecordsInChunks(ctx context.Context, tx *sql.Tx, records []metadata.ItemRecord) error {
	for start := 0; start < len(records); start += scopeInsertChunkSize {
		end := start + scopeInsertChunkSize
		if end > len(records) {
			end = len(records)
		}
		if err := metadata.InsertItems(ctx, tx, records[start:end]); err != nil {
			return err
		}
	}
	return nil
}
