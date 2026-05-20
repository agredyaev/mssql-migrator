package fs

const (
	KindCodeUnknown   uint8 = 0
	KindCodeTypes     uint8 = 1
	KindCodeSequences uint8 = 2
	KindCodeTables    uint8 = 3
	KindCodeSynonyms  uint8 = 4
	KindCodeIndexes   uint8 = 5
	KindCodeViews     uint8 = 6
	KindCodeFunctions uint8 = 7
	KindCodeProcedures uint8 = 8
	KindCodeTriggers  uint8 = 9
)

var objectKindCodes = map[string]uint8{
	"types":      KindCodeTypes,
	"sequences":  KindCodeSequences,
	"tables":     KindCodeTables,
	"synonyms":   KindCodeSynonyms,
	"indexes":    KindCodeIndexes,
	"views":      KindCodeViews,
	"functions":  KindCodeFunctions,
	"procedures": KindCodeProcedures,
	"triggers":   KindCodeTriggers,
}

func ObjectKindCode(kind string) uint8 {
	return objectKindCodes[kind]
}

func IsModuleKindCode(code uint8) bool {
	switch code {
	case KindCodeViews, KindCodeFunctions, KindCodeProcedures, KindCodeTriggers:
		return true
	default:
		return false
	}
}
