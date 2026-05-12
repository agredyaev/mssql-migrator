package parser

import "strings"

func ManagedSchemaNames(layout Layout) []string {
	if len(layout.Schemas) == 0 {
		return nil
	}
	result := make([]string, 0, len(layout.Schemas))
	for _, schema := range layout.Schemas {
		if strings.TrimSpace(schema.Name) == "" {
			continue
		}
		result = append(result, schema.Name)
	}
	return result
}

func ManagedObjectKeys(layout Layout) []string {
	if len(layout.Objects) == 0 {
		return nil
	}
	result := make([]string, 0, len(layout.Objects))
	for _, object := range layout.Objects {
		if strings.TrimSpace(object.NormalizedKey) == "" {
			continue
		}
		result = append(result, object.NormalizedKey)
	}
	return result
}
