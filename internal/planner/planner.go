package planner

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"reporting-db-migrations/internal/catalog"
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

type CatalogObject = catalog.Object

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
		return parser.Layout{}, "", contracts.Wrap(contracts.ErrInvalidInput, err)
	}
	return layout, parser.HashLayout(layout, false), nil
}

func ResolvePlanningLayoutForRunner(cfg config.Config) (parser.Layout, string, error) {
	return resolvePlanningLayout(cfg)
}

func VerifyApprovedPlan(cfg config.Config, current contracts.MigrationPlan) error {
	p, err := reports.ReadPlan(cfg.PlanFile)
	if err != nil {
		return contracts.Wrap(contracts.ErrApprovedPlanMissing, err)
	}
	if p.Blocked {
		return fmt.Errorf("%w: approved plan is blocked", contracts.ErrApprovedPlanMismatch)
	}
	if p.SchemaVersion != "v8" {
		return fmt.Errorf("%w: schema version %s", contracts.ErrApprovedPlanMismatch, p.SchemaVersion)
	}
	if p.Command != "plan" {
		return fmt.Errorf("%w: command %s", contracts.ErrApprovedPlanMismatch, p.Command)
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
		return contracts.Wrap(contracts.ErrApprovedPlanMismatch, fmt.Errorf("%v", mm))
	}
	if !reflect.DeepEqual(stableSchemas(p.Schemas), stableSchemas(current.Schemas)) {
		return fmt.Errorf("%w: schema set does not match current deployment state", contracts.ErrApprovedPlanMismatch)
	}
	if !reflect.DeepEqual(stableObjects(p.Objects), stableObjects(current.Objects)) {
		return fmt.Errorf("%w: object set does not match current deployment state", contracts.ErrApprovedPlanMismatch)
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
	state := CatalogState{Schemas: map[string]struct{}{}, Objects: map[string]CatalogObject{}, SuccessfulByKey: map[string]string{}}
	catalogState, err := catalog.Read(ctx, r.conn)
	if err != nil {
		return CatalogState{}, err
	}
	state.Schemas = catalogState.Schemas
	state.Objects = catalogState.Objects
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
		Command:           contracts.CommandPlan,
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
		Rollback:        contracts.RollbackScope(cfg.TransactionMode),
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
		action := contracts.SchemaActionCreateSchema
		if exists {
			action = contracts.SchemaActionExists
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
			TransactionMode: contracts.TransactionModeForObject(plan.TransactionMode, object.NoTransaction),
			RollbackScope:   contracts.RollbackScopeForObject(plan.TransactionMode, object.NoTransaction),
			NoTransaction:   contracts.NoTransactionForObject(plan.TransactionMode, object.NoTransaction),
			SourceFile:      object.Path,
		}
		_, exists := catalog.Objects[object.NormalizedKey]
		planned.Exists = exists
		planned.PlannedAction = determineObjectAction(object, catalog, updatePolicy)
		if isUnsafeUpdateAction(planned.PlannedAction) && !parser.SupportsExistingObjectUpdate(object) {
			planned.PlannedAction = contracts.ActionReprocessChangedBlocked
			plan.BlockReasons = append(plan.BlockReasons, "existing object update requires CREATE OR ALTER: "+object.Path)
		}
		metadataMatch := inferMetadataMatch(object, catalog)
		if metadataMatch != nil {
			planned.MetadataMatch = metadataMatch
		}
		plan.Objects = append(plan.Objects, planned)
		switch planned.PlannedAction {
		case contracts.ActionCreateObject:
			plan.Summary.CreateCount++
		case contracts.ActionAdoptExisting:
			plan.Summary.AdoptCount++
		case contracts.ActionSkipUnchanged:
			plan.Summary.SkipCount++
		case contracts.ActionReprocessChangedBlocked:
			if !containsBlockReason(plan.BlockReasons, "existing object update requires CREATE OR ALTER: "+object.Path) {
				plan.BlockReasons = append(plan.BlockReasons, "existing object changed: "+object.Path)
			}
			plan.Summary.ChangedCount++
		case contracts.ActionUpdateExistingModule, contracts.ActionUpdateExistingSupported:
			plan.Summary.ChangedCount++
		case contracts.ActionFail:
			plan.Failures = append(plan.Failures, "invalid object state: "+object.Path)
		}
	}
	sort.Slice(plan.Objects, func(i, j int) bool {
		return plan.Objects[i].NormalizedKey < plan.Objects[j].NormalizedKey
	})
}

func isUnsafeUpdateAction(action string) bool {
	return action == contracts.ActionUpdateExistingModule || action == contracts.ActionUpdateExistingSupported
}

func containsBlockReason(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func determineObjectAction(object parser.Object, catalog CatalogState, updatePolicy string) string {
	_, exists := catalog.Objects[object.NormalizedKey]
	if !exists {
		return contracts.ActionCreateObject
	}
	checksum, tracked := catalog.SuccessfulByKey[object.NormalizedKey]
	if tracked && checksum == object.Checksum {
		return contracts.ActionSkipUnchanged
	}
	if tracked && checksum != object.Checksum {
		if parser.IsModuleKind(object.Kind) && updatePolicy == config.UpdatePolicyModulesOnly {
			return contracts.ActionUpdateExistingModule
		}
		if parser.IsModuleKind(object.Kind) && updatePolicy == config.UpdatePolicyAllSupported {
			return contracts.ActionUpdateExistingSupported
		}
		return contracts.ActionReprocessChangedBlocked
	}
	return contracts.ActionAdoptExisting
}

func inferMetadataMatch(object parser.Object, catalog CatalogState) *bool {
	checksum, ok := catalog.SuccessfulByKey[object.NormalizedKey]
	if !ok {
		return nil
	}
	matched := checksum == object.Checksum
	return &matched
}
