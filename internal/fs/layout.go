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

type cachedFile struct {
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

func (c *cachedFile) loadContent(absPath string) (string, error) {
	c.contentOnce.Do(func() {
		data, err := os.ReadFile(absPath)
		if err != nil {
			c.contentErr = err
			return
		}
		c.content = string(data)
	})
	return c.content, c.contentErr
}

func (c *cachedFile) loadChecksum(absPath string) (string, error) {
	c.checksumOnce.Do(func() {
		content, err := c.loadContent(absPath)
		if err != nil {
			c.checksumErr = err
			return
		}
		c.checksum = NormalizeAndHash(content)
	})
	return c.checksum, c.checksumErr
}

func (c *cachedFile) loadGitInfo(absPath string) {
	c.gitOnce.Do(func() {
		c.gitHash, c.gitAuthor, c.gitDate, c.gitErr = gitInfo(absPath)
	})
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

	file cachedFile
}

func (o *Object) Content() (string, error)  { return o.file.loadContent(o.AbsolutePath) }
func (o *Object) Checksum() (string, error) { return o.file.loadChecksum(o.AbsolutePath) }
func (o *Object) GitHash() (string, error) {
	o.file.loadGitInfo(o.AbsolutePath)
	return o.file.gitHash, o.file.gitErr
}
func (o *Object) GitAuthor() (string, error) {
	o.file.loadGitInfo(o.AbsolutePath)
	return o.file.gitAuthor, o.file.gitErr
}
func (o *Object) GitDate() (string, error) {
	o.file.loadGitInfo(o.AbsolutePath)
	return o.file.gitDate, o.file.gitErr
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

	file cachedFile
}

func (ts *TransitionScript) Content() (string, error)  { return ts.file.loadContent(ts.AbsolutePath) }
func (ts *TransitionScript) Checksum() (string, error) { return ts.file.loadChecksum(ts.AbsolutePath) }
func (ts *TransitionScript) GitHash() (string, error) {
	ts.file.loadGitInfo(ts.AbsolutePath)
	return ts.file.gitHash, ts.file.gitErr
}
func (ts *TransitionScript) GitAuthor() (string, error) {
	ts.file.loadGitInfo(ts.AbsolutePath)
	return ts.file.gitAuthor, ts.file.gitErr
}
func (ts *TransitionScript) GitDate() (string, error) {
	ts.file.loadGitInfo(ts.AbsolutePath)
	return ts.file.gitDate, ts.file.gitErr
}

type CheckScript struct {
	Path          string
	AbsolutePath  string
	DatabaseName  string
	SchemaName    string
	Name          string
	NoTransaction bool

	file cachedFile
}

func (cs *CheckScript) Content() (string, error)  { return cs.file.loadContent(cs.AbsolutePath) }
func (cs *CheckScript) Checksum() (string, error) { return cs.file.loadChecksum(cs.AbsolutePath) }

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

func (l *Layout) ObjectsByPath() map[string]*Object {
	m := make(map[string]*Object, len(l.Objects))
	for _, obj := range l.Objects {
		m[obj.Path] = obj
	}
	return m
}

func (l *Layout) TransitionsByPath() map[string]*TransitionScript {
	m := make(map[string]*TransitionScript, len(l.Transitions))
	for _, ts := range l.Transitions {
		m[ts.Path] = ts
	}
	return m
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
