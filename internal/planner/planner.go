package planner

import (
	"context"
	"sort"
	"strings"
	"time"

	"reporting-db-migrations/internal/catalog"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
)

type CatalogReader interface {
	ReadCatalogState(context.Context) (CatalogState, error)
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

func BuildWithCatalog(ctx context.Context, cfg config.Config, successfulByKey map[string]string, reader CatalogReader) (contracts.MigrationPlan, error) {
	layout, hash, err := resolvePlanningLayout(cfg)
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	return BuildResolved(ctx, cfg, successfulByKey, layout, hash, reader)
}

func BuildResolvedWithCatalog(cfg config.Config, successfulByKey map[string]string, layout parser.Layout, hash string, catalogState CatalogState) (contracts.MigrationPlan, error) {
	state := normalizedCatalogState(catalogState, successfulByKey)
	return buildResolvedWithCatalogState(cfg, layout, hash, state), nil
}

func BuildResolved(ctx context.Context, cfg config.Config, successfulByKey map[string]string, layout parser.Layout, hash string, reader CatalogReader) (contracts.MigrationPlan, error) {
	catalog, err := loadCatalogState(ctx, reader, successfulByKey)
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	return BuildResolvedWithCatalog(cfg, successfulByKey, layout, hash, catalog)
}

func buildResolvedWithCatalogState(cfg config.Config, layout parser.Layout, hash string, catalog CatalogState) contracts.MigrationPlan {
	plan := newPlan(cfg, hash, layout)
	planSchemas(&plan, layout, catalog)
	planObjects(&plan, layout, catalog, cfg.UpdatePolicy)
	plan.Summary.CheckCount = len(layout.Checks)
	plan.Summary.FailureCount = len(plan.Failures)
	plan.Blockers = append(append([]string{}, plan.BlockReasons...), plan.Failures...)
	plan.Blocked = len(plan.BlockReasons) > 0 || len(plan.Failures) > 0
	if plan.Blocked {
		plan.Summary.BlockedCount = len(plan.BlockReasons) + len(plan.Failures)
	}
	return plan
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

func loadCatalogState(ctx context.Context, reader CatalogReader, successfulByKey map[string]string) (CatalogState, error) {
	if reader == nil {
		return normalizedCatalogState(CatalogState{}, successfulByKey), nil
	}
	state, err := reader.ReadCatalogState(ctx)
	if err != nil {
		return CatalogState{}, err
	}
	return normalizedCatalogState(state, successfulByKey), nil
}

func normalizedCatalogState(state CatalogState, successfulByKey map[string]string) CatalogState {
	if state.SuccessfulByKey == nil {
		state.SuccessfulByKey = cloneChecksums(successfulByKey)
	} else {
		merged := cloneChecksums(state.SuccessfulByKey)
		for key, checksum := range successfulByKey {
			if _, exists := merged[key]; !exists {
				merged[key] = checksum
			}
		}
		state.SuccessfulByKey = merged
	}
	if state.Schemas == nil {
		state.Schemas = map[string]struct{}{}
	}
	if state.Objects == nil {
		state.Objects = map[string]CatalogObject{}
	}
	return state
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
	transitionsByKey := make(map[string][]parser.TransitionScript)
	for _, transition := range layout.Transitions {
		transitionsByKey[transition.NormalizedKey] = append(transitionsByKey[transition.NormalizedKey], transition)
	}
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
			NoTransaction:   noTransactionForObject(plan.TransactionMode, object.NoTransaction),
			SourceFile:      object.Path,
		}
		transitions := transitionsByKey[object.NormalizedKey]
		planned.TransitionPaths = transitionPaths(transitions)
		_, exists := catalog.Objects[object.NormalizedKey]
		planned.Exists = exists
		planned.PlannedAction = determineObjectAction(object, catalog, updatePolicy, parser.HasExecutableTransition(transitions))
		if isUnsafeUpdateAction(planned.PlannedAction) && !parser.SupportsExistingObjectUpdate(object) {
			planned.PlannedAction = contracts.ActionReprocessChangedBlocked
			plan.BlockReasons = append(plan.BlockReasons, existingObjectUpdateBlockReason(object))
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
			if !containsBlockReason(plan.BlockReasons, existingObjectUpdateBlockReason(object)) {
				plan.BlockReasons = append(plan.BlockReasons, blockedExistingObjectReason(object, transitions))
			}
			plan.Summary.ChangedCount++
		case contracts.ActionReprocessChanged:
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

func determineObjectAction(object parser.Object, catalog CatalogState, updatePolicy string, hasTransition bool) string {
	_, exists := catalog.Objects[object.NormalizedKey]
	if !exists {
		return contracts.ActionCreateObject
	}
	checksum, tracked := catalog.SuccessfulByKey[object.NormalizedKey]
	if tracked && checksum == object.Checksum {
		return contracts.ActionSkipUnchanged
	}
	if tracked && checksum != object.Checksum {
		if object.Kind == "tables" && hasTransition {
			return contracts.ActionReprocessChanged
		}
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

func transitionPaths(items []parser.TransitionScript) []string {
	if len(items) == 0 {
		return nil
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}
	return paths
}

func blockedExistingObjectReason(object parser.Object, transitions []parser.TransitionScript) string {
	if object.Kind == "tables" {
		if len(transitions) == 0 {
			return "tracked table drift detected for " + object.Path + ". Add a checked-in migration under " + object.SchemaName + "/tables/_migrations/" + object.ObjectName + "/, rerun plan, then run migrate."
		}
		for _, transition := range transitions {
			if transition.Scaffold {
				return "tracked table drift detected for " + object.Path + ". Replace the scaffold at " + transition.Path + " with real SQL, rerun plan, then run migrate."
			}
		}
		return "tracked table drift detected for " + object.Path + ". Make sure the checked-in migration files under " + object.SchemaName + "/tables/_migrations/" + object.ObjectName + "/ are executable, then rerun plan."
	}
	return existingObjectUpdateBlockReason(object)
}

func existingObjectUpdateBlockReason(object parser.Object) string {
	kindName := strings.TrimSuffix(strings.ToUpper(object.Kind), "S")
	return "tracked " + strings.TrimSpace(strings.ToLower(kindName)) + " drift detected for " + object.Path + ". Update the repo file to start with CREATE OR ALTER " + kindName + ", then rerun plan and migrate."
}

func inferMetadataMatch(object parser.Object, catalog CatalogState) *bool {
	checksum, ok := catalog.SuccessfulByKey[object.NormalizedKey]
	if !ok {
		return nil
	}
	matched := checksum == object.Checksum
	return &matched
}
