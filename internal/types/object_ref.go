package types

type ObjectRef struct {
	NormalizedKey string
	ObjectPath    string
	SchemaName    string
	Kind          string
	ObjectName    string
}

type GitInfo struct {
	GitHash   string
	GitAuthor string
	GitDate   string
}
