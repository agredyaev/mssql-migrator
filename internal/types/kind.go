package types

const (
	KindTables     = "tables"
	KindViews      = "views"
	KindProcedures = "procedures"
	KindFunctions  = "functions"
	KindTriggers   = "triggers"
	KindIndexes    = "indexes"
	KindTypes      = "types"
	KindSequences  = "sequences"
	KindSynonyms   = "synonyms"
)

var kindIsTransactional = map[string]bool{
	KindTables:     true,
	KindIndexes:    true,
	KindTypes:      true,
	KindSequences:  true,
	KindSynonyms:   true,
	KindViews:      false,
	KindProcedures: false,
	KindFunctions:  false,
	KindTriggers:   false,
}

func IsModuleKind(kind string) bool {
	tx, ok := kindIsTransactional[kind]
	return ok && !tx
}

func IsNoTransactionKind(kind string) bool {
	return IsModuleKind(kind)
}

func IsTransactionalKind(kind string) bool {
	tx, ok := kindIsTransactional[kind]
	return ok && tx
}
