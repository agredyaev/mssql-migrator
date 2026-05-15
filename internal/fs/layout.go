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

type CachedFile struct {
	AbsPath      string
	gitInfoFn    func(string) (string, string, string, error)
	contentOnce  sync.Once
	checksumOnce sync.Once
	gitOnce      sync.Once
	content      string
	checksum     string
	gitHash      string
	gitAuthor    string
	gitDate      string
	contentErr   error
	checksumErr  error
	gitErr       error
}

func (c *CachedFile) Content() (string, error) {
	c.contentOnce.Do(func() {
		data, err := os.ReadFile(c.AbsPath)
		if err != nil {
			c.contentErr = err
			return
		}
		c.content = string(data)
	})
	return c.content, c.contentErr
}

func (c *CachedFile) Checksum() (string, error) {
	c.checksumOnce.Do(func() {
		content, err := c.Content()
		if err != nil {
			c.checksumErr = err
			return
		}
		c.checksum = NormalizeAndHash(content)
	})
	return c.checksum, c.checksumErr
}

func (c *CachedFile) loadGitInfo() {
	c.gitOnce.Do(func() {
		if c.gitInfoFn == nil {
			c.gitErr = fmt.Errorf("git info not available for %s", c.AbsPath)
			return
		}
		c.gitHash, c.gitAuthor, c.gitDate, c.gitErr = c.gitInfoFn(c.AbsPath)
	})
}

func (c *CachedFile) GitHash() (string, error) {
	c.loadGitInfo()
	return c.gitHash, c.gitErr
}

func (c *CachedFile) GitAuthor() (string, error) {
	c.loadGitInfo()
	return c.gitAuthor, c.gitErr
}

func (c *CachedFile) GitDate() (string, error) {
	c.loadGitInfo()
	return c.gitDate, c.gitErr
}

type Object struct {
	Path                 string
	DatabaseName         string
	SchemaName           string
	NormalizedSchemaName string
	Kind                 string
	ObjectName           string
	ParentName           string
	NormalizedKey        string
	NoTransaction        bool

	CachedFile
}

type TransitionScript struct {
	Path          string
	DatabaseName  string
	SchemaName    string
	TableName     string
	NormalizedKey string
	Ordinal       string
	Commit        string
	Slug          string
	NoTransaction bool
	Scaffold      bool

	CachedFile
}

type CheckScript struct {
	Path          string
	DatabaseName  string
	SchemaName    string
	Name          string
	NoTransaction bool

	CachedFile
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
