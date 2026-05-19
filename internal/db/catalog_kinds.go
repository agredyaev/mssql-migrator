package db

import "reporting-db-migrations/internal/fs"

type catalogKinds struct {
	sysObjects bool
	types      bool
	indexes    bool
}

func catalogKindsForLayout(scope fs.Layout) catalogKinds {
	var k catalogKinds
	for _, obj := range scope.Objects {
		switch obj.Kind {
		case "types":
			k.types = true
		case "indexes":
			k.indexes = true
		default:
			k.sysObjects = true
		}
	}
	return k
}
