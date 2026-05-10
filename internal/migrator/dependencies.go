package migrator

import (
	"strings"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
)

type objectDependencyResolver struct{}

func (objectDependencyResolver) ParentCandidates(schemaName string, parentName string) []string {
	parentName = strings.ToLower(strings.TrimSpace(parentName))
	schemaName = strings.ToLower(strings.TrimSpace(schemaName))
	if parentName == "" {
		return nil
	}
	return []string{
		schemaName + "/tables/" + parentName,
		schemaName + "/views/" + parentName,
	}
}

func (r objectDependencyResolver) ParentSatisfied(plannedByKey map[string]contracts.PlannedObject, object parser.Object) bool {
	if strings.TrimSpace(object.ParentName) == "" {
		return true
	}
	for _, key := range r.ParentCandidates(object.SchemaName, object.ParentName) {
		planned, ok := plannedByKey[key]
		if !ok {
			continue
		}
		switch planned.PlannedAction {
		case contracts.ActionCreateObject, contracts.ActionAdoptExisting, contracts.ActionSkipUnchanged:
			return true
		}
	}
	return false
}
