package fs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Layout struct {
	RootPath    string
	Schemas     []Schema
	Objects     []*Object
	Transitions []*TransitionScript
	Checks      []*CheckScript
}

type Schema struct {
	DatabaseName   string
	Name           string
	NormalizedName string
}

type Object struct {
	Path                 string
	AbsolutePath         string
	DatabaseName         string
	SchemaName           string
	NormalizedSchemaName string
	Kind                 string
	ObjectName           string
	ParentName           string
	NormalizedKey        string
	NoTransaction        bool

	contentOnce  sync.Once
	content      string
	contentErr   error
	checksumOnce sync.Once
	checksum     string
	checksumErr  error
	gitOnce      sync.Once
	gitHash      string
	gitAuthor    string
	gitDate      string
	gitErr       error
}

func (o *Object) Content() (string, error) {
	o.contentOnce.Do(func() {
		data, err := os.ReadFile(o.AbsolutePath)
		if err != nil {
			o.contentErr = err
			return
		}
		o.content = string(data)
	})
	return o.content, o.contentErr
}

func (o *Object) Checksum() (string, error) {
	o.checksumOnce.Do(func() {
		content, err := o.Content()
		if err != nil {
			o.checksumErr = err
			return
		}
		o.checksum = NormalizeAndHash(content)
	})
	return o.checksum, o.checksumErr
}

func (o *Object) GitHash() (string, error) {
	o.gitOnce.Do(o.loadGitInfo)
	return o.gitHash, o.gitErr
}

func (o *Object) GitAuthor() (string, error) {
	o.gitOnce.Do(o.loadGitInfo)
	return o.gitAuthor, o.gitErr
}

func (o *Object) GitDate() (string, error) {
	o.gitOnce.Do(o.loadGitInfo)
	return o.gitDate, o.gitErr
}

func (o *Object) loadGitInfo() {
	o.gitHash, o.gitAuthor, o.gitDate, o.gitErr = gitInfo(o.AbsolutePath)
}

type TransitionScript struct {
	Path          string
	AbsolutePath  string
	DatabaseName  string
	SchemaName    string
	TableName     string
	NormalizedKey string
	Ordinal       string
	Commit        string
	Slug          string
	NoTransaction bool
	Scaffold      bool

	contentOnce  sync.Once
	content      string
	contentErr   error
	checksumOnce sync.Once
	checksum     string
	checksumErr  error
	gitOnce      sync.Once
	gitHash      string
	gitAuthor    string
	gitDate      string
	gitErr       error
}

func (ts *TransitionScript) Content() (string, error) {
	ts.contentOnce.Do(func() {
		data, err := os.ReadFile(ts.AbsolutePath)
		if err != nil {
			ts.contentErr = err
			return
		}
		ts.content = string(data)
	})
	return ts.content, ts.contentErr
}

func (ts *TransitionScript) Checksum() (string, error) {
	ts.checksumOnce.Do(func() {
		content, err := ts.Content()
		if err != nil {
			ts.checksumErr = err
			return
		}
		ts.checksum = NormalizeAndHash(content)
	})
	return ts.checksum, ts.checksumErr
}

func (ts *TransitionScript) GitHash() (string, error) {
	ts.gitOnce.Do(ts.loadGitInfo)
	return ts.gitHash, ts.gitErr
}

func (ts *TransitionScript) GitAuthor() (string, error) {
	ts.gitOnce.Do(ts.loadGitInfo)
	return ts.gitAuthor, ts.gitErr
}

func (ts *TransitionScript) GitDate() (string, error) {
	ts.gitOnce.Do(ts.loadGitInfo)
	return ts.gitDate, ts.gitErr
}

func (ts *TransitionScript) loadGitInfo() {
	ts.gitHash, ts.gitAuthor, ts.gitDate, ts.gitErr = gitInfo(ts.AbsolutePath)
}

type CheckScript struct {
	Path          string
	AbsolutePath  string
	DatabaseName  string
	SchemaName    string
	Name          string
	NoTransaction bool

	contentOnce  sync.Once
	content      string
	contentErr   error
	checksumOnce sync.Once
	checksum     string
	checksumErr  error
}

func (cs *CheckScript) Content() (string, error) {
	cs.contentOnce.Do(func() {
		data, err := os.ReadFile(cs.AbsolutePath)
		if err != nil {
			cs.contentErr = err
			return
		}
		cs.content = string(data)
	})
	return cs.content, cs.contentErr
}

func (cs *CheckScript) Checksum() (string, error) {
	cs.checksumOnce.Do(func() {
		content, err := cs.Content()
		if err != nil {
			cs.checksumErr = err
			return
		}
		cs.checksum = NormalizeAndHash(content)
	})
	return cs.checksum, cs.checksumErr
}

func NormalizeAndHash(input string) string {
	normalized := normalizeSQL(input)
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func (l *Layout) NormalizedKeys() []string {
	keys := make([]string, len(l.Objects))
	for i, obj := range l.Objects {
		keys[i] = obj.NormalizedKey
	}
	return keys
}

func (l *Layout) HasExecutableTransition() bool {
	for _, ts := range l.Transitions {
		if !ts.Scaffold {
			return true
		}
	}
	return false
}

func (l *Layout) LayoutHash() (string, error) {
	h := sha256.New()
	checksums := make([]string, 0, len(l.Objects)+len(l.Transitions))

	for _, obj := range l.Objects {
		cs, err := obj.Checksum()
		if err != nil {
			return "", err
		}
		checksums = append(checksums, cs)
	}
	for _, ts := range l.Transitions {
		cs, err := ts.Checksum()
		if err != nil {
			return "", err
		}
		checksums = append(checksums, cs)
	}

	sort.Strings(checksums)
	for _, cs := range checksums {
		h.Write([]byte(cs))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func gitInfo(absPath string) (hash, author, date string, err error) {
	fileDir := filepath.Dir(absPath)
	fileName := filepath.Base(absPath)
	cmd := exec.Command("git", "-C", fileDir, "log", "-1", "--format=%H%n%an%n%aI", "--", fileName)
	out, err := cmd.Output()
	if err != nil {
		return "", "", "", fmt.Errorf("git log %s: %w", fileName, err)
	}
	lines := strings.SplitN(strings.TrimSpace(string(out)), "\n", 3)
	if len(lines) < 3 {
		return "", "", "", fmt.Errorf("git log %s: unexpected output: %s", fileName, string(out))
	}
	return lines[0], lines[1], lines[2], nil
}
