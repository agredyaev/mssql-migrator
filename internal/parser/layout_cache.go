package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"reporting-db-migrations/internal/checksum"
)

const layoutManifestVersion = 1

type lazySQLContent struct {
	label            string
	path             string
	expectedChecksum string
	expectedSize     int64
	expectedModTime  int64

	mu      sync.Mutex
	loaded  bool
	content string
}

type layoutManifest struct {
	Version  int                            `json:"version"`
	RootPath string                         `json:"root_path"`
	Entries  map[string]layoutManifestEntry `json:"entries"`
}

type layoutManifestEntry struct {
	Size                   int64  `json:"size"`
	ModTimeUnixNano        int64  `json:"mod_time_unix_nano"`
	Checksum               string `json:"checksum"`
	NoTransaction          bool   `json:"no_transaction,omitempty"`
	SupportsExistingUpdate bool   `json:"supports_existing_update,omitempty"`
	Scaffold               bool   `json:"scaffold,omitempty"`
}

type sqlFileMetadata struct {
	checksum               string
	noTransaction          bool
	supportsExistingUpdate bool
	scaffold               bool
	contentState           *lazySQLContent
	manifestEntry          layoutManifestEntry
}

func newLazySQLContent(label string, path string, expectedChecksum string, expectedSize int64, expectedModTime int64) *lazySQLContent {
	return &lazySQLContent{label: label, path: path, expectedChecksum: expectedChecksum, expectedSize: expectedSize, expectedModTime: expectedModTime}
}

func (c *lazySQLContent) read() (string, error) {
	if c == nil {
		return "", nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded {
		matches, err := c.matchesCurrentFile()
		if err != nil {
			return "", err
		}
		if matches {
		return c.content, nil
		}
	}
	body, err := os.ReadFile(c.path)
	if err != nil {
		return "", err
	}
	content := string(body)
	actualChecksum := checksum.SHA256String(content)
	if c.expectedChecksum != "" && actualChecksum != c.expectedChecksum {
		label := strings.TrimSpace(c.label)
		if label == "" {
			label = filepath.ToSlash(c.path)
		}
		return "", fmt.Errorf("repo layout changed after discovery: %s checksum changed; rerun the command", label)
	}
	if info, err := os.Stat(c.path); err == nil {
		c.expectedSize = info.Size()
		c.expectedModTime = info.ModTime().UnixNano()
	}
	c.content = content
	c.loaded = true
	return c.content, nil
}

func (c *lazySQLContent) matchesCurrentFile() (bool, error) {
	if c == nil || strings.TrimSpace(c.path) == "" {
		return true, nil
	}
	info, err := os.Stat(c.path)
	if err != nil {
		return false, err
	}
	return info.Size() == c.expectedSize && info.ModTime().UnixNano() == c.expectedModTime, nil
}

func discoverSQLFileMetadata(rel string, abs string, kind string, info fs.FileInfo, cached layoutManifestEntry) (sqlFileMetadata, error) {
	if cached.matches(info) {
		return sqlFileMetadata{
			checksum:               cached.Checksum,
			noTransaction:          cached.NoTransaction,
			supportsExistingUpdate: cached.SupportsExistingUpdate,
			scaffold:               cached.Scaffold,
			contentState:           newLazySQLContent(rel, abs, cached.Checksum, cached.Size, cached.ModTimeUnixNano),
			manifestEntry:          cached,
		}, nil
	}
	content, fileChecksum, err := readSQLFileWithChecksum(abs)
	if err != nil {
		return sqlFileMetadata{}, err
	}
	entry := layoutManifestEntry{
		Size:                   info.Size(),
		ModTimeUnixNano:        info.ModTime().UnixNano(),
		Checksum:               fileChecksum,
		NoTransaction:          hasNoTransactionDirective(content),
		SupportsExistingUpdate: supportsExistingObjectUpdateContent(kind, content),
		Scaffold:               hasTransitionScaffoldDirective(content),
	}
	return sqlFileMetadata{
		checksum:               fileChecksum,
		noTransaction:          entry.NoTransaction,
		supportsExistingUpdate: entry.SupportsExistingUpdate,
		scaffold:               entry.Scaffold,
		contentState:           newLazySQLContent(rel, abs, fileChecksum, entry.Size, entry.ModTimeUnixNano),
		manifestEntry:          entry,
	}, nil
}

func loadLayoutManifest(root string) layoutManifest {
	path, err := layoutManifestPath(root)
	if err != nil {
		return layoutManifest{}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return layoutManifest{}
	}
	var manifest layoutManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return layoutManifest{}
	}
	if manifest.Version != layoutManifestVersion || manifest.RootPath != normalizedLayoutManifestRoot(root) || len(manifest.Entries) == 0 {
		return layoutManifest{}
	}
	return manifest
}

func writeLayoutManifest(root string, entries map[string]layoutManifestEntry) {
	if len(entries) == 0 {
		return
	}
	path, err := layoutManifestPath(root)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	body, err := json.Marshal(layoutManifest{Version: layoutManifestVersion, RootPath: normalizedLayoutManifestRoot(root), Entries: entries})
	if err != nil {
		return
	}
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, body, 0o644); err != nil {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tempPath)
		return
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
	}
}

func layoutManifestPath(root string) (string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(cacheRoot) == "" {
		cacheRoot = os.TempDir()
	}
	if strings.TrimSpace(cacheRoot) == "" {
		return "", fmt.Errorf("resolve layout cache dir")
	}
	sum := sha256.Sum256([]byte(normalizedLayoutManifestRoot(root)))
	return filepath.Join(cacheRoot, "rmig", "layout-manifests", hex.EncodeToString(sum[:])+".json"), nil
}

func normalizedLayoutManifestRoot(root string) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return filepath.Clean(root)
	}
	return filepath.Clean(absRoot)
}

func supportsExistingObjectUpdateContent(kind string, content string) bool {
	if !IsModuleKind(kind) {
		return false
	}
	trimmed := strings.TrimSpace(strings.TrimPrefix(content, "\ufeff"))
	upper := strings.ToUpper(trimmed)
	switch kind {
	case "views":
		return strings.HasPrefix(upper, "CREATE OR ALTER VIEW")
	case "procedures":
		return strings.HasPrefix(upper, "CREATE OR ALTER PROCEDURE") || strings.HasPrefix(upper, "CREATE OR ALTER PROC")
	case "functions":
		return strings.HasPrefix(upper, "CREATE OR ALTER FUNCTION")
	case "triggers":
		return strings.HasPrefix(upper, "CREATE OR ALTER TRIGGER")
	default:
		return false
	}
}

func (e layoutManifestEntry) matches(info fs.FileInfo) bool {
	return e.Checksum != "" && e.Size == info.Size() && e.ModTimeUnixNano == info.ModTime().UnixNano()
}
