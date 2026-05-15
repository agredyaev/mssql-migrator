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

var moduleKinds = map[string]bool{
	KindViews: true, KindProcedures: true, KindFunctions: true, KindTriggers: true,
}

func IsModuleKind(kind string) bool {
	return moduleKinds[kind]
}

var transactionalKinds = map[string]bool{
	KindTables: true, KindIndexes: true, KindTypes: true, KindSequences: true, KindSynonyms: true,
}

func IsTransactionalKind(kind string) bool {
	return transactionalKinds[kind]
}

func IsNoTransactionKind(kind string) bool {
	return moduleKinds[kind]
}
