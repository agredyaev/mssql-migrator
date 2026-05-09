package planner

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/reports"
)

type CatalogReader interface {
	ReadCatalogState(context.Context) (CatalogState, error)
}

type sqlCatalogReader struct {
	conn *sql.Conn
}

func SQLCatalogReader(conn *sql.Conn) CatalogReader {
	return sqlCatalogReader{conn: conn}
}

type CatalogState struct {
	Schemas         map[string]struct{}
	Objects         map[string]CatalogObject
	SuccessfulByKey map[string]string
}

type CatalogObject struct {
	SchemaName string
	Kind       string
	ObjectName string
	ParentName string
}

const catalogStateQuery = `
SELECT s.name, o.type_desc, o.name, ISNULL(parent.name, '')
FROM sys.objects o
JOIN sys.schemas s ON s.schema_id = o.schema_id
LEFT JOIN sys.objects parent ON parent.object_id = o.parent_object_id
WHERE o.is_ms_shipped = 0
UNION ALL
SELECT s.name, 'USER_TABLE_TYPE', tt.name, ''
FROM sys.table_types tt
JOIN sys.schemas s ON s.schema_id = tt.schema_id
UNION ALL
SELECT s.name, 'INDEX', i.name, o.name
FROM sys.indexes i
JOIN sys.objects o ON o.object_id = i.object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
WHERE i.is_hypothetical = 0 AND i.name IS NOT NULL AND o.type IN ('U', 'V') AND o.is_ms_shipped = 0
ORDER BY 1, 3`

func Build(cfg config.Config, successfulByKey map[string]string) (contracts.MigrationPlan, error) {
	layout, hash, err := resolvePlanningLayout(cfg)
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	return BuildResolved(context.Background(), cfg, successfulByKey, layout, hash, nil)
}

func BuildWithConnection(ctx context.Context, cfg config.Config, successfulByKey map[string]string, conn *sql.Conn) (contracts.MigrationPlan, error) {
	layout, hash, err := resolvePlanningLayout(cfg)
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	return BuildResolved(ctx, cfg, successfulByKey, layout, hash, sqlCatalogReader{conn: conn})
}

func BuildWithCatalog(ctx context.Context, cfg config.Config, successfulByKey map[string]string, reader CatalogReader) (contracts.MigrationPlan, error) {
	layout, hash, err := resolvePlanningLayout(cfg)
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	return BuildResolved(ctx, cfg, successfulByKey, layout, hash, reader)
}

func BuildResolved(ctx context.Context, cfg config.Config, successfulByKey map[string]string, layout parser.Layout, hash string, reader CatalogReader) (contracts.MigrationPlan, error) {
	plan := newPlan(cfg, hash, layout)
	catalog, err := loadCatalogState(ctx, reader, successfulByKey)
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	planSchemas(&plan, layout, catalog)
	planObjects(&plan, layout, catalog, cfg.UpdatePolicy)
	plan.Summary.CheckCount = len(layout.Checks)
	plan.Summary.FailureCount = len(plan.Failures)
	plan.Blockers = append(append([]string{}, plan.BlockReasons...), plan.Failures...)
	plan.Blocked = len(plan.BlockReasons) > 0 || len(plan.Failures) > 0
	if plan.Blocked {
		plan.Summary.BlockedCount = len(plan.BlockReasons) + len(plan.Failures)
	}
	return plan, nil
}

func resolvePlanningLayout(cfg config.Config) (parser.Layout, string, error) {
	layout, err := parser.DiscoverLayout(cfg.SelectedBasePath())
	if err != nil {
		return parser.Layout{}, "", fmt.Errorf("%w: %v", contracts.ErrInvalidInput, err)
	}
	return layout, parser.HashLayout(layout, false), nil
}

func ResolvePlanningLayoutForRunner(cfg config.Config) (parser.Layout, string, error) {
	return resolvePlanningLayout(cfg)
}

func VerifyApprovedPlan(cfg config.Config, current contracts.MigrationPlan) error {
	p, err := reports.ReadPlan(cfg.PlanFile)
	if err != nil {
		return fmt.Errorf("approved plan missing: %w", err)
	}
	if p.Blocked {
		return fmt.Errorf("approved plan mismatch: approved plan is blocked")
	}
	if p.SchemaVersion != "v8" {
		return fmt.Errorf("approved plan mismatch: schema version %s", p.SchemaVersion)
	}
	if p.Command != "plan" {
		return fmt.Errorf("approved plan mismatch: command %s", p.Command)
	}
	mm := []string{}
	if p.GitCommit != cfg.GitCommit {
		mm = append(mm, "git_commit")
	}
	if p.LayoutHash != current.LayoutHash {
		mm = append(mm, "layout_hash")
	}
	if p.Target.Environment != current.Target.Environment {
		mm = append(mm, "target.environment")
	}
	if p.Target.Database != current.Target.Database {
		mm = append(mm, "target.database")
	}
	if p.ToolVersion != current.ToolVersion {
		mm = append(mm, "tool_version")
	}
	if p.ToolCommit != current.ToolCommit {
		mm = append(mm, "tool_commit")
	}
	if p.ComparisonMode != current.ComparisonMode {
		mm = append(mm, "comparison_mode")
	}
	if p.UpdatePolicy != current.UpdatePolicy {
		mm = append(mm, "update_policy")
	}
	if p.TransactionMode != current.TransactionMode {
		mm = append(mm, "transaction_mode")
	}
	if p.Rollback != current.Rollback {
		mm = append(mm, "rollback")
	}
	if p.SQLRoot != current.SQLRoot {
		mm = append(mm, "sql_root")
	}
	if p.Base != current.Base {
		mm = append(mm, "base")
	}
	if p.EffectiveBasePath != current.EffectiveBasePath {
		mm = append(mm, "effective_base_path")
	}
	if len(mm) > 0 {
		return fmt.Errorf("approved plan mismatch: %v", mm)
	}
	if !reflect.DeepEqual(stableSchemas(p.Schemas), stableSchemas(current.Schemas)) {
		return fmt.Errorf("approved plan mismatch: schema set does not match current deployment state")
	}
	if !reflect.DeepEqual(stableObjects(p.Objects), stableObjects(current.Objects)) {
		return fmt.Errorf("approved plan mismatch: object set does not match current deployment state")
	}
	return nil
}

func stableSchemas(items []contracts.PlannedSchema) []contracts.PlannedSchema {
	result := make([]contracts.PlannedSchema, len(items))
	copy(result, items)
	sort.Slice(result, func(i, j int) bool {
		left := strings.ToLower(result[i].SchemaName)
		right := strings.ToLower(result[j].SchemaName)
		if left != right {
			return left < right
		}
		if result[i].Action != result[j].Action {
			return result[i].Action < result[j].Action
		}
		return !result[i].Exists && result[j].Exists
	})
	return result
}

func stableObjects(items []contracts.PlannedObject) []contracts.PlannedObject {
	result := make([]contracts.PlannedObject, 0, len(items))
	for _, item := range items {
		result = append(result, contracts.PlannedObject{
			ObjectPath:      item.ObjectPath,
			SchemaName:      item.SchemaName,
			Kind:            item.Kind,
			ObjectName:      item.ObjectName,
			ParentName:      item.ParentName,
			NormalizedKey:   item.NormalizedKey,
			Checksum:        item.Checksum,
			PlannedAction:   item.PlannedAction,
			TransactionMode: item.TransactionMode,
			RollbackScope:   item.RollbackScope,
			NoTransaction:   item.NoTransaction,
			SourceFile:      item.SourceFile,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].NormalizedKey != result[j].NormalizedKey {
			return result[i].NormalizedKey < result[j].NormalizedKey
		}
		return result[i].ObjectPath < result[j].ObjectPath
	})
	return result
}

func (r sqlCatalogReader) ReadCatalogState(ctx context.Context) (CatalogState, error) {
	state := CatalogState{
		Schemas:         map[string]struct{}{},
		Objects:         map[string]CatalogObject{},
		SuccessfulByKey: map[string]string{},
	}
	if r.conn == nil {
		return state, nil
	}
	schemaRows, err := r.conn.QueryContext(ctx, `SELECT name FROM sys.schemas WHERE name NOT IN ('sys', 'INFORMATION_SCHEMA') ORDER BY name`)
	if err != nil {
		return CatalogState{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	for schemaRows.Next() {
		var schemaName string
		if err := schemaRows.Scan(&schemaName); err != nil {
			schemaRows.Close()
			return CatalogState{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
		}
		state.Schemas[strings.ToLower(schemaName)] = struct{}{}
	}
	if err := schemaRows.Err(); err != nil {
		schemaRows.Close()
		return CatalogState{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	schemaRows.Close()

	rows, err := r.conn.QueryContext(ctx, catalogStateQuery)
	if err != nil {
		return CatalogState{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	defer rows.Close()
	for rows.Next() {
		var schemaName string
		var typeDesc string
		var objectName string
		var parentName string
		if err := rows.Scan(&schemaName, &typeDesc, &objectName, &parentName); err != nil {
			return CatalogState{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
		}
		kind := mapTypeDescToKind(typeDesc)
		if kind == "" {
			continue
		}
		key := strings.ToLower(schemaName) + "/" + kind + "/"
		if parentName != "" && (kind == "triggers" || kind == "indexes") {
			key += strings.ToLower(parentName) + "/"
		}
		key += strings.ToLower(objectName)
		state.Objects[key] = CatalogObject{SchemaName: schemaName, Kind: kind, ObjectName: objectName, ParentName: parentName}
	}
	if err := rows.Err(); err != nil {
		return CatalogState{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	return state, nil
}

func loadCatalogState(ctx context.Context, reader CatalogReader, successfulByKey map[string]string) (CatalogState, error) {
	if reader == nil {
		return CatalogState{Schemas: map[string]struct{}{}, Objects: map[string]CatalogObject{}, SuccessfulByKey: cloneChecksums(successfulByKey)}, nil
	}
	catalog, err := reader.ReadCatalogState(ctx)
	if err != nil {
		return CatalogState{}, err
	}
	if catalog.SuccessfulByKey == nil {
		catalog.SuccessfulByKey = cloneChecksums(successfulByKey)
	} else {
		for key, checksum := range successfulByKey {
			if _, exists := catalog.SuccessfulByKey[key]; !exists {
				catalog.SuccessfulByKey[key] = checksum
			}
		}
	}
	if catalog.Schemas == nil {
		catalog.Schemas = map[string]struct{}{}
	}
	if catalog.Objects == nil {
		catalog.Objects = map[string]CatalogObject{}
	}
	return catalog, nil
}

func cloneChecksums(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func newPlan(cfg config.Config, hash string, layout parser.Layout) contracts.MigrationPlan {
	return contracts.MigrationPlan{
		SchemaVersion:     "v8",
		Command:           "plan",
		Tool:              "rmig",
		ToolVersion:       cfg.ToolVersion,
		ToolCommit:        cfg.ToolCommit,
		GitCommit:         cfg.GitCommit,
		GitBranch:         cfg.GitBranch,
		SQLRoot:           cfg.SQLRoot,
		Base:              cfg.SQLBase,
		EffectiveBasePath: cfg.SelectedBasePath(),
		LayoutHash:        hash,
		Target: contracts.PlanTarget{
			Environment: cfg.Env,
			Database:    cfg.Database,
		},
		ComparisonMode:  cfg.ComparisonMode,
		UpdatePolicy:    cfg.UpdatePolicy,
		TransactionMode: cfg.TransactionMode,
		Rollback:        rollbackScope(cfg.TransactionMode),
		PlannedAt:       time.Now().UTC(),
		Summary: contracts.PlanSummary{
			SchemaCount: len(layout.Schemas),
			ObjectCount: len(layout.Objects),
		},
		Schemas:      []contracts.PlannedSchema{},
		Objects:      []contracts.PlannedObject{},
		Failures:     []string{},
		Blockers:     []string{},
		BlockReasons: []string{},
	}
}

func planSchemas(plan *contracts.MigrationPlan, layout parser.Layout, catalog CatalogState) {
	for _, schema := range layout.Schemas {
		_, exists := catalog.Schemas[schema.NormalizedName]
		action := "create_schema"
		if exists {
			action = "exists"
		}
		plan.Schemas = append(plan.Schemas, contracts.PlannedSchema{
			SchemaName: schema.Name,
			Action:     action,
			Exists:     exists,
		})
		if exists {
			plan.Summary.SkipCount++
			continue
		}
		plan.Summary.CreateCount++
	}
}

func planObjects(plan *contracts.MigrationPlan, layout parser.Layout, catalog CatalogState, updatePolicy string) {
	for _, object := range layout.Objects {
		planned := contracts.PlannedObject{
			ObjectPath:      object.Path,
			SchemaName:      object.SchemaName,
			Kind:            object.Kind,
			ObjectName:      object.ObjectName,
			ParentName:      object.ParentName,
			NormalizedKey:   object.NormalizedKey,
			Checksum:        object.Checksum,
			TransactionMode: transactionModeForObject(plan.TransactionMode, object.NoTransaction),
			RollbackScope:   rollbackScopeForObject(plan.TransactionMode, object.NoTransaction),
			NoTransaction:   object.NoTransaction,
			SourceFile:      object.Path,
		}
		_, exists := catalog.Objects[object.NormalizedKey]
		planned.Exists = exists
		planned.PlannedAction = determineObjectAction(object, catalog, updatePolicy)
		metadataMatch := inferMetadataMatch(object, catalog)
		if metadataMatch != nil {
			planned.MetadataMatch = metadataMatch
		}
		plan.Objects = append(plan.Objects, planned)
		switch planned.PlannedAction {
		case "create_object":
			plan.Summary.CreateCount++
		case "adopt_existing":
			plan.Summary.AdoptCount++
		case "skip_unchanged":
			plan.Summary.SkipCount++
		case "reprocess_changed_blocked":
			plan.BlockReasons = append(plan.BlockReasons, "existing object changed: "+object.Path)
			plan.Summary.ChangedCount++
		case "update_existing_module", "update_existing_supported":
			plan.Summary.ChangedCount++
		case "fail":
			plan.Failures = append(plan.Failures, "invalid object state: "+object.Path)
		}
	}
	sort.Slice(plan.Objects, func(i, j int) bool {
		return plan.Objects[i].NormalizedKey < plan.Objects[j].NormalizedKey
	})
}

func determineObjectAction(object parser.Object, catalog CatalogState, updatePolicy string) string {
	_, exists := catalog.Objects[object.NormalizedKey]
	if !exists {
		return "create_object"
	}
	checksum, tracked := catalog.SuccessfulByKey[object.NormalizedKey]
	if tracked && checksum == object.Checksum {
		return "skip_unchanged"
	}
	if tracked && checksum != object.Checksum {
		if parser.IsModuleKind(object.Kind) && updatePolicy == config.UpdatePolicyModulesOnly {
			return "update_existing_module"
		}
		if parser.IsModuleKind(object.Kind) && updatePolicy == config.UpdatePolicyAllSupported {
			return "update_existing_supported"
		}
		return "reprocess_changed_blocked"
	}
	return "adopt_existing"
}

func inferMetadataMatch(object parser.Object, catalog CatalogState) *bool {
	checksum, ok := catalog.SuccessfulByKey[object.NormalizedKey]
	if !ok {
		return nil
	}
	matched := checksum == object.Checksum
	return &matched
}

func transactionModeForObject(defaultMode string, noTransaction bool) string {
	if noTransaction {
		return config.TransactionModeNone
	}
	return defaultMode
}

func rollbackScope(defaultMode string) string {
	if defaultMode == config.TransactionModeNone {
		return "none"
	}
	return "script"
}

func rollbackScopeForObject(defaultMode string, noTransaction bool) string {
	if noTransaction || defaultMode == config.TransactionModeNone {
		return "none"
	}
	return "script"
}

func mapTypeDescToKind(typeDesc string) string {
	switch strings.ToUpper(strings.TrimSpace(typeDesc)) {
	case "USER_TABLE":
		return "tables"
	case "VIEW":
		return "views"
	case "SQL_STORED_PROCEDURE":
		return "procedures"
	case "SQL_SCALAR_FUNCTION", "SQL_INLINE_TABLE_VALUED_FUNCTION", "SQL_TABLE_VALUED_FUNCTION":
		return "functions"
	case "SQL_TRIGGER":
		return "triggers"
	case "INDEX":
		return "indexes"
	case "USER_TABLE_TYPE":
		return "types"
	case "SEQUENCE_OBJECT":
		return "sequences"
	case "SYNONYM":
		return "synonyms"
	default:
		return ""
	}
}
