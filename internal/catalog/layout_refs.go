package catalog

import "reporting-db-migrations/internal/parser"

func ObjectRefsForLayout(layout parser.Layout) []ObjectRef {
	if len(layout.Objects) == 0 {
		return nil
	}
	result := make([]ObjectRef, 0, len(layout.Objects))
	for _, object := range layout.Objects {
		result = append(result, ObjectRef{
			SchemaName: object.SchemaName,
			Kind:       object.Kind,
			ParentName: object.ParentName,
			ObjectName: object.ObjectName,
		})
	}
	return result
}
