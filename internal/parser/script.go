package parser

type ScriptType string

const (
	ScriptTypeVersioned  ScriptType = "versioned"
	ScriptTypeRepeatable ScriptType = "repeatable"
	ScriptTypeCheck      ScriptType = "check"
)

type Script struct {
	Name          string
	Path          string
	Type          ScriptType
	Version       string
	Description   string
	Checksum      string
	NoTransaction bool
}

type Batch struct {
	SQL    string
	Repeat int
}
