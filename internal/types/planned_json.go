package types

import "encoding/json"

// plannedObjectWire matches the historical JSON shape of PlannedObject when Git
// metadata was embedded (flat GitHash / GitAuthor / GitDate next to ObjectRef).
type plannedObjectWire struct {
	MetadataMatch *bool
	ObjectRef
	TransactionMode string
	ParentName      string
	PlannedAction   string
	DatabaseName    string
	RollbackScope   string
	GitHash         string
	GitAuthor       string
	GitDate         string
	TransitionPaths []string
	Checksum        [32]byte
	Exists          bool
	NoTransaction   bool
}

// MarshalJSON preserves the flat git field layout used by .plan.json consumers.
func (p PlannedObject) MarshalJSON() ([]byte, error) {
	w := plannedObjectWire{
		ObjectRef:       p.ObjectRef,
		TransitionPaths: p.TransitionPaths,
		DatabaseName:    p.DatabaseName,
		ParentName:      p.ParentName,
		Checksum:        p.Checksum,
		PlannedAction:   p.PlannedAction,
		TransactionMode: p.TransactionMode,
		RollbackScope:   p.RollbackScope,
		MetadataMatch:   p.MetadataMatch,
		Exists:          p.Exists,
		NoTransaction:   p.NoTransaction,
	}
	if p.Git != nil {
		w.GitHash = p.Git.GitHash
		w.GitAuthor = p.Git.GitAuthor
		w.GitDate = p.Git.GitDate
	}
	return json.Marshal(w)
}

// UnmarshalJSON restores optional in-memory Git from the legacy flat JSON layout.
func (p *PlannedObject) UnmarshalJSON(data []byte) error {
	var w plannedObjectWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	p.ObjectRef = w.ObjectRef
	p.TransitionPaths = w.TransitionPaths
	p.DatabaseName = w.DatabaseName
	p.ParentName = w.ParentName
	p.Checksum = w.Checksum
	p.PlannedAction = w.PlannedAction
	p.TransactionMode = w.TransactionMode
	p.RollbackScope = w.RollbackScope
	p.MetadataMatch = w.MetadataMatch
	p.Exists = w.Exists
	p.NoTransaction = w.NoTransaction
	if w.GitHash != "" || w.GitAuthor != "" || w.GitDate != "" {
		g := GitInfo{GitHash: w.GitHash, GitAuthor: w.GitAuthor, GitDate: w.GitDate}
		p.Git = &g
	} else {
		p.Git = nil
	}
	return nil
}

// GitStrings returns git metadata for SQL apply / history; empty when Git is nil.
func (p PlannedObject) GitStrings() (hash, author, date string) {
	if p.Git == nil {
		return "", "", ""
	}
	return p.Git.GitHash, p.Git.GitAuthor, p.Git.GitDate
}
