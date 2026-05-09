package validate

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/catalog"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
)

type objectRef struct {
	Schema string
	Name   string
	Kind   string
}

type CatalogState struct {
	Schemas map[string]struct{}
	Objects map[string]CatalogObject
}

type CatalogObject = catalog.Object

const catalogStateQuery = catalog.StateQuery

func ReadCatalogState(ctx context.Context, conn *sql.Conn) (CatalogState, error) {
	state, err := catalog.Read(ctx, conn)
	if err != nil {
		return CatalogState{}, err
	}
	return CatalogState{Schemas: state.Schemas, Objects: state.Objects}, nil
}

func RefreshManagedObjects(ctx context.Context, conn *sql.Conn, layout parser.Layout, log logger.Logger) (contracts.ValidationSummary, error) {
	catalog, err := ReadCatalogState(ctx, conn)
	if err != nil {
		return contracts.ValidationSummary{}, err
	}
	objects, missing := managedScopeRefs(layout.Objects, catalog.Objects)

	summary := contracts.ValidationSummary{}
	if len(missing) > 0 {
		return summary, fmt.Errorf("missing managed objects: %s", strings.Join(missing, ", "))
	}
	for _, expected := range layout.Objects {
		if !parser.IsModuleKind(expected.Kind) {
			continue
		}
		object := objects[expected.NormalizedKey]
		qualifiedName := bracket(object.Schema) + "." + bracket(object.Name)
		if _, err := conn.ExecContext(ctx, "EXEC sys.sp_refreshsqlmodule @p1", qualifiedName); err != nil {
			return summary, fmt.Errorf("refresh module %s: %w", qualifiedName, err)
		}
		if object.Kind == "views" {
			if _, err := conn.ExecContext(ctx, "EXEC sys.sp_refreshview @p1", qualifiedName); err != nil {
				return summary, fmt.Errorf("refresh view %s: %w", qualifiedName, err)
			}
		}
		summary.ModulesRefreshed++
	}

	log.Info("modules_refreshed", fmt.Sprintf("count=%d", summary.ModulesRefreshed))
	return summary, nil
}

func RunChecks(ctx context.Context, conn *sql.Conn, layout parser.Layout) (contracts.ValidationSummary, error) {
	summary := contracts.ValidationSummary{}
	for _, script := range layout.Checks {
		if err := runCheck(ctx, conn, script); err != nil {
			summary.ChecksFailed++
			return summary, fmt.Errorf("check %s failed: %w", script.Path, err)
		}
		summary.ChecksPassed++
	}
	return summary, nil
}

func LoadLayout(cfg config.Config) (parser.Layout, error) {
	return parser.DiscoverValidationLayout(cfg.SelectedBasePath())
}

func runCheck(ctx context.Context, conn *sql.Conn, script parser.CheckScript) error {
	batches, err := parser.SplitGO(script.Content)
	if err != nil {
		return err
	}
	for _, batch := range batches {
		for i := 0; i < batch.Repeat; i++ {
			if _, err := conn.ExecContext(ctx, batch.SQL); err != nil {
				return err
			}
		}
	}
	return nil
}

func managedScopeRefs(expected []parser.Object, actual map[string]CatalogObject) (map[string]objectRef, []string) {
	result := map[string]objectRef{}
	missing := []string{}
	for _, object := range expected {
		catalogObject, ok := actual[object.NormalizedKey]
		if !ok {
			missing = append(missing, object.Path)
			continue
		}
		result[object.NormalizedKey] = objectRef{
			Schema: catalogObject.SchemaName,
			Name:   catalogObject.ObjectName,
			Kind:   catalogObject.Kind,
		}
	}
	return result, missing
}

func bracket(value string) string {
	return "[" + strings.ReplaceAll(value, "]", "]]") + "]"
}

func mapTypeDescToKind(typeDesc string) string {
	return catalog.MapTypeDescToKind(typeDesc)
}
