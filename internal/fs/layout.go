package fs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

var layoutHashDigestsPool = sync.Pool{
	New: func() any {
		s := make([][32]byte, 0, 64)
		return &s
	},
}

type Layout struct {
	RootPath    string
	Schemas     []Schema
	Objects     []*Object
	Transitions []*TransitionScript
	Checks      []*CheckScript

	// objectsByPath and transitionsByPath are derived from Objects / Transitions.
	// Scanner.Scan ends with rebuildPathIndexes so Apply avoids rebuilding maps.
	// If you mutate Objects or Transitions after the first index build, call
	// RebuildPathIndexes before lookups.
	objectsByPath     map[string]*Object
	transitionsByPath map[string]*TransitionScript
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
	checksumDone uint32
	contentBytes []byte
	content      string
	checksum     [32]byte
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
		c.contentBytes = data
		if len(data) == 0 {
			c.content = ""
			return
		}
		// Keep the os.ReadFile buffer on CachedFile so Content and Checksum can
		// share the same bytes without a second full string copy.
		c.content = unsafe.String(unsafe.SliceData(data), len(data))
	})
	return c.content, c.contentErr
}

func (c *CachedFile) Checksum() ([32]byte, error) {
	c.checksumOnce.Do(func() {
		content, err := c.Content()
		if err != nil {
			c.checksumErr = err
			return
		}
		c.checksum = NormalizeAndHash(content)
		atomic.StoreUint32(&c.checksumDone, 1)
	})
	return c.checksum, c.checksumErr
}

func (c *CachedFile) IsChecksumCached() bool {
	return atomic.LoadUint32(&c.checksumDone) == 1
}

func (c *CachedFile) preloadChecksum(sum [32]byte) {
	c.checksumOnce.Do(func() {
		c.checksum = sum
		atomic.StoreUint32(&c.checksumDone, 1)
	})
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

func (c *CachedFile) preloadGitInfo(hash, author, date string) {
	c.gitOnce.Do(func() {
		c.gitHash = hash
		c.gitAuthor = author
		c.gitDate = date
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
	CachedFile

	Path                 string
	DatabaseName         string
	SchemaName           string
	NormalizedSchemaName string
	Kind                 string
	ObjectName           string
	ParentName           string
	NormalizedKey        string
	NoTransaction        bool
}

type TransitionScript struct {
	CachedFile

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
}

type CheckScript struct {
	CachedFile

	Path          string
	DatabaseName  string
	SchemaName    string
	Name          string
	NoTransaction bool
}

func NormalizeAndHash(input string) [32]byte {
	bufPtr := normalizePool.Get().(*[]byte)
	b := (*bufPtr)[:0]

	b = normalizeSQLBytes(input, b)
	sum := sha256.Sum256(b)

	*bufPtr = b
	normalizePool.Put(bufPtr)

	return sum
}

func (l *Layout) NormalizedKeys() []string {
	keys := make([]string, len(l.Objects))
	for i, obj := range l.Objects {
		keys[i] = obj.NormalizedKey
	}
	return keys
}

func buildObjectsByPath(objs []*Object) map[string]*Object {
	m := make(map[string]*Object, len(objs))
	for _, obj := range objs {
		m[obj.Path] = obj
	}
	return m
}

func buildTransitionsByPath(trans []*TransitionScript) map[string]*TransitionScript {
	m := make(map[string]*TransitionScript, len(trans))
	for _, ts := range trans {
		m[ts.Path] = ts
	}
	return m
}

// RebuildPathIndexes refreshes path lookup maps from the current Objects and
// Transitions slices. Scanner.Scan calls this after scanning; tests that append
// to Objects or Transitions after Scan should call it before ObjectsByPath /
// TransitionsByPath.
func (l *Layout) RebuildPathIndexes() {
	l.objectsByPath = buildObjectsByPath(l.Objects)
	l.transitionsByPath = buildTransitionsByPath(l.Transitions)
}

func (l *Layout) ObjectsByPath() map[string]*Object {
	if l.objectsByPath == nil {
		l.objectsByPath = buildObjectsByPath(l.Objects)
	}
	return l.objectsByPath
}

func (l *Layout) TransitionsByPath() map[string]*TransitionScript {
	if l.transitionsByPath == nil {
		l.transitionsByPath = buildTransitionsByPath(l.Transitions)
	}
	return l.transitionsByPath
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
	p := layoutHashDigestsPool.Get().(*[][32]byte)
	raw := (*p)[:0]
	need := len(l.Objects) + len(l.Transitions)
	if cap(raw) < need {
		raw = make([][32]byte, 0, need)
	}
	defer func() {
		*p = raw[:0]
		layoutHashDigestsPool.Put(p)
	}()

	for _, obj := range l.Objects {
		cs, err := obj.Checksum()
		if err != nil {
			return "", err
		}
		raw = append(raw, cs)
	}
	for _, ts := range l.Transitions {
		cs, err := ts.Checksum()
		if err != nil {
			return "", err
		}
		raw = append(raw, cs)
	}

	sort.Slice(raw, func(i, j int) bool {
		return bytes.Compare(raw[i][:], raw[j][:]) < 0
	})
	for i := range raw {
		h.Write(raw[i][:])
	}

	var out [64]byte
	hex.Encode(out[:], h.Sum(nil))
	return string(out[:]), nil
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
