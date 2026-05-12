package validate

import (
	"fmt"
	"strings"

	"reporting-db-migrations/internal/parser"
)

type Scope struct {
	Objects []parser.Object
	Missing []parser.Object
	Refs    map[string]objectRef
}

func ResolveManagedScope(layout parser.Layout, catalog CatalogState) (Scope, error) {
	refs, missingPaths := managedScopeRefs(layout.Objects, catalog.Objects)
	missingByKey := make(map[string]struct{}, len(missingPaths))
	for _, object := range layout.Objects {
		if _, ok := refs[object.NormalizedKey]; ok {
			continue
		}
		missingByKey[object.NormalizedKey] = struct{}{}
	}
	scope := Scope{Objects: layout.Objects, Missing: make([]parser.Object, 0, len(missingByKey)), Refs: refs}
	for _, object := range layout.Objects {
		if _, ok := missingByKey[object.NormalizedKey]; ok {
			scope.Missing = append(scope.Missing, object)
		}
	}
	if len(scope.Missing) == 0 {
		return scope, nil
	}
	return scope, fmt.Errorf("missing managed objects: %s", strings.Join(missingPaths, ", "))
}

func (s Scope) ExistingObjects() []parser.Object {
	if len(s.Missing) == 0 {
		return s.Objects
	}
	result := make([]parser.Object, 0, len(s.Objects)-len(s.Missing))
	for _, object := range s.Objects {
		if _, ok := s.Refs[object.NormalizedKey]; !ok {
			continue
		}
		result = append(result, object)
	}
	return result
}
