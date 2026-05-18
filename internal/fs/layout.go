package fs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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
	// Scanner.Scan ends with RebuildPathIndexes so Apply avoids rebuilding maps.
	// If you mutate Objects or Transitions after the first index build, call
	// RebuildPathIndexes before lookups.
	objectsByPath     map[string]*Object
	transitionsByPath map[string]*TransitionScript

	// retainObjectOrder and nonObjectOrder are permutations sorted by AbsPath
	// (RebuildPathIndexes). They partition SQL files into objects (checksum may
	// retain raw bytes for Content) vs transitions/checks (checksum-only by
	// default) without duplicating path strings. Policy is enforced in code via
	// (*Object).Checksum vs (*CachedFile).Checksum.
	retainObjectOrder []int
	nonObjectOrder    []layoutNonObjectSlot
}

// layoutNonObjectSlot indexes either Transitions or Checks for sorted-path metadata.
type layoutNonObjectSlot struct {
	transition bool // true = Transitions[i], false = Checks[i]
	i          int
}

type Schema struct {
	DatabaseName   string
	Name           string
	NormalizedName string
}

type CachedFile struct {
	contentErr   error
	gitErr       error
	checksumErr  error
	gitInfoFn    func(string) (string, string, string, error)
	gitDate      string
	content      string
	gitHash      string
	gitAuthor    string
	AbsPath      string
	contentBytes []byte
	gitOnce      sync.Once
	checksumOnce sync.Once
	contentOnce  sync.Once
	contentDone  uint32
	checksumDone uint32
	checksum     [32]byte
}

func (c *CachedFile) Content() (string, error) {
	c.contentOnce.Do(func() {
		if c.contentBytes != nil {
			if len(c.contentBytes) == 0 {
				c.content = ""
			} else {
				c.content = unsafe.String(unsafe.SliceData(c.contentBytes), len(c.contentBytes))
			}
			atomic.StoreUint32(&c.contentDone, 1)
			return
		}
		data, err := os.ReadFile(c.AbsPath)
		if err != nil {
			c.contentErr = err
			atomic.StoreUint32(&c.contentDone, 1)
			return
		}
		c.contentBytes = data
		if len(data) == 0 {
			c.content = ""
			atomic.StoreUint32(&c.contentDone, 1)
			return
		}
		// Keep the os.ReadFile buffer on CachedFile so Content and Checksum can
		// share the same bytes without a second full string copy.
		c.content = unsafe.String(unsafe.SliceData(data), len(data))
		atomic.StoreUint32(&c.contentDone, 1)
	})
	return c.content, c.contentErr
}

func (c *CachedFile) Checksum() ([32]byte, error) {
	return checksumOnceForFile(c, false, nil)
}

func (o *Object) Checksum() ([32]byte, error) {
	var hint *scanPathState
	if o.objectStatForByteCacheValid {
		hint = &o.objectStatForByteCache
	}
	return checksumOnceForFile(&o.CachedFile, true, hint)
}

func checksumOnceForFile(c *CachedFile, retainContentBytes bool, objectStatHint *scanPathState) ([32]byte, error) {
	c.checksumOnce.Do(func() {
		if atomic.LoadUint32(&c.contentDone) == 1 {
			if c.contentErr != nil {
				c.checksumErr = c.contentErr
				return
			}
			if c.contentBytes != nil {
				c.checksum = NormalizeAndHashBytes(c.contentBytes)
				atomic.StoreUint32(&c.checksumDone, 1)
				return
			}
		}
		if retainContentBytes {
			if data, ok := lookupSharedObjectBytes(c.AbsPath, objectStatHint); ok {
				c.contentBytes = data
				c.checksum = NormalizeAndHashBytes(data)
				atomic.StoreUint32(&c.checksumDone, 1)
				return
			}
		}
		if retainContentBytes {
			data, st, err := readFileBytesAndStat(c.AbsPath)
			if err != nil {
				c.checksumErr = err
				return
			}
			c.contentBytes = data
			storeSharedObjectBytesWithStat(c.AbsPath, data, st)
			c.checksum = NormalizeAndHashBytes(data)
			atomic.StoreUint32(&c.checksumDone, 1)
			return
		}
		data, err := os.ReadFile(c.AbsPath)
		if err != nil {
			c.checksumErr = err
			return
		}
		c.checksum = NormalizeAndHashBytes(data)
		atomic.StoreUint32(&c.checksumDone, 1)
	})
	return c.checksum, c.checksumErr
}

func readFileBytesAndStat(path string) ([]byte, scanPathState, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, scanPathState{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, scanPathState{}, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, scanPathState{}, err
	}
	return data, scanPathState{size: info.Size(), modTime: info.ModTime().UnixNano()}, nil
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
	Path                 string
	DatabaseName         string
	SchemaName           string
	NormalizedSchemaName string
	Kind                 string
	ObjectName           string
	ParentName           string
	// ParentNormalizedKey is types.NormalizedKey(SchemaName, "tables", ParentName)
	// when Kind=="triggers" and ParentName is non-empty; set in Scanner.newObject.
	// Empty when not applicable. Used to avoid recomputing the parent map key in diff.
	ParentNormalizedKey string
	NormalizedKey       string
	CachedFile
	objectStatForByteCache      scanPathState
	NoTransaction               bool
	objectStatForByteCacheValid bool
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
	CachedFile
	NoTransaction bool
	Scaffold      bool
}

type CheckScript struct {
	Path         string
	DatabaseName string
	SchemaName   string
	Name         string
	CachedFile
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

func NormalizeAndHashBytes(input []byte) [32]byte {
	bufPtr := normalizePool.Get().(*[]byte)
	b := (*bufPtr)[:0]

	if len(input) > 0 {
		b = normalizeSQLBytes(unsafe.String(unsafe.SliceData(input), len(input)), b)
	}
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
	l.rebuildContentRetainPathLists()
}

func (l *Layout) rebuildContentRetainPathLists() {
	l.retainObjectOrder = l.retainObjectOrder[:0]
	if cap(l.retainObjectOrder) < len(l.Objects) {
		l.retainObjectOrder = make([]int, 0, len(l.Objects))
	}
	for i, o := range l.Objects {
		if o != nil {
			l.retainObjectOrder = append(l.retainObjectOrder, i)
		}
	}
	sort.Slice(l.retainObjectOrder, func(a, b int) bool {
		ia, ib := l.retainObjectOrder[a], l.retainObjectOrder[b]
		return l.Objects[ia].AbsPath < l.Objects[ib].AbsPath
	})

	l.nonObjectOrder = l.nonObjectOrder[:0]
	need := len(l.Transitions) + len(l.Checks)
	if cap(l.nonObjectOrder) < need {
		l.nonObjectOrder = make([]layoutNonObjectSlot, 0, need)
	}
	for i := range l.Transitions {
		if l.Transitions[i] != nil {
			l.nonObjectOrder = append(l.nonObjectOrder, layoutNonObjectSlot{transition: true, i: i})
		}
	}
	for i := range l.Checks {
		if l.Checks[i] != nil {
			l.nonObjectOrder = append(l.nonObjectOrder, layoutNonObjectSlot{transition: false, i: i})
		}
	}
	sort.Slice(l.nonObjectOrder, func(a, b int) bool {
		return l.nonObjectAbsPath(&l.nonObjectOrder[a]) < l.nonObjectAbsPath(&l.nonObjectOrder[b])
	})
}

func (l *Layout) nonObjectAbsPath(s *layoutNonObjectSlot) string {
	if s.transition {
		return l.Transitions[s.i].AbsPath
	}
	return l.Checks[s.i].AbsPath
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
