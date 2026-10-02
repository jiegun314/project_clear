// Package config owns the YAML-backed runtime parameters.
//
// A default parameter file ships with the binary. On startup, if the user's
// parameter directory is empty, the default is copied there so the file always
// exists on disk and can be hand-edited.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// HeaderDisplay controls how the two-row week header is rendered in the UI.
type HeaderDisplay string

const (
	// HeaderTwoRow renders the week code and its start date on two lines,
	// mirroring rows 55/56 of the source workbook.
	HeaderTwoRow HeaderDisplay = "twoRow"
	// HeaderOneRow renders "2639 / 2026-09-21" on a single line.
	HeaderOneRow HeaderDisplay = "oneRow"
)

// ExportMode selects the export engine.
type ExportMode string

const (
	// ExportClean rebuilds a workbook that holds only the MPS page: the header
	// block plus every merged row, colours and notes included. This is the
	// default — 纯数据.
	ExportClean ExportMode = "clean"
	// ExportTemplate rewrites a stored copy of the first source workbook, so
	// the original file format (styles, conditional formats, macros) survives,
	// but every sheet other than MPS is dropped.
	ExportTemplate ExportMode = "template"
)

// Config is the full parameter set exposed on the settings screen.
type Config struct {
	// ConfigVersion is the schema marker used to bring an older settings file
	// forward exactly once.
	ConfigVersion int `yaml:"configVersion" json:"configVersion"`
	// ReadColumns is how many week columns to read starting at column P.
	ReadColumns int `yaml:"readColumns" json:"readColumns"`
	// LOCFilter is the value required in the LOC column (column L) for a row
	// to be kept.
	LOCFilter string `yaml:"locFilter" json:"locFilter"`
	// PageSize is the row count per page in the main data grid.
	PageSize int `yaml:"pageSize" json:"pageSize"`
	// HeaderDisplay is twoRow (default) or oneRow.
	HeaderDisplay HeaderDisplay `yaml:"headerDisplay" json:"headerDisplay"`
	// ExportMode is clean (纯数据, default) or template (原文件格式).
	ExportMode ExportMode `yaml:"exportMode" json:"exportMode"`
	// ExportDir remembers the last used export directory.
	ExportDir string `yaml:"exportDir" json:"exportDir"`
}

// currentConfigVersion is bumped whenever a shipped default changes in a way an
// existing settings file should follow.
//
// v2: the default export mode became 纯数据 (clean).
const currentConfigVersion = 2

// Default returns the built-in parameter set, matching the specification.
func Default() Config {
	return Config{
		ConfigVersion: currentConfigVersion,
		ReadColumns:   20,
		LOCFilter:     "WH_CNB",
		PageSize:      200,
		HeaderDisplay: HeaderTwoRow,
		ExportMode:    ExportClean,
	}
}

// Validate clamps values into sane ranges and reports what it had to fix.
func (c *Config) Validate() []string {
	var notes []string
	if c.ReadColumns < 1 || c.ReadColumns > 200 {
		notes = append(notes, fmt.Sprintf("读取列数 %d 超出范围，已重置为 20", c.ReadColumns))
		c.ReadColumns = 20
	}
	if c.PageSize < 10 || c.PageSize > 5000 {
		notes = append(notes, fmt.Sprintf("每页行数 %d 超出范围，已重置为 200", c.PageSize))
		c.PageSize = 200
	}
	if c.HeaderDisplay != HeaderTwoRow && c.HeaderDisplay != HeaderOneRow {
		notes = append(notes, fmt.Sprintf("表头显示模式 %q 无效，已重置为 twoRow", c.HeaderDisplay))
		c.HeaderDisplay = HeaderTwoRow
	}
	if c.ExportMode != ExportTemplate && c.ExportMode != ExportClean {
		notes = append(notes, fmt.Sprintf("导出模式 %q 无效，已重置为 clean", c.ExportMode))
		c.ExportMode = ExportClean
	}
	c.LOCFilter = trimSpace(c.LOCFilter)
	if c.LOCFilter == "" {
		notes = append(notes, "LOC 筛选值不能为空，已重置为 WH_CNB")
		c.LOCFilter = "WH_CNB"
	}
	return notes
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// Store loads and persists Config, keeping the file and an in-memory copy in
// sync.
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

// NewStore resolves the parameter directory, seeds it from the embedded
// default when empty, and loads the result.
func NewStore() (*Store, []string, error) {
	dir, err := configDir()
	if err != nil {
		return nil, nil, err
	}
	return NewStoreAt(dir)
}

// NewStoreAt loads (or seeds) the parameter file in a specific folder. The
// application uses NewStore, which resolves the folder next to the program;
// this entry point is for tools and tests that keep their own scratch space.
func NewStoreAt(dir string) (*Store, []string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("创建参数目录失败: %w", err)
	}
	path := filepath.Join(dir, "clear.yaml")
	var notes []string
	// A settings file left in an older location wins over the shipped default.
	if from, err := AdoptDir("config", "clear.yaml", dir, LegacyBases()); err != nil {
		notes = append(notes, fmt.Sprintf("迁移旧参数文件失败: %v", err))
	} else if from != "" {
		notes = append(notes, fmt.Sprintf("已从旧位置迁移参数文件: %s", from))
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, defaultYAML, 0o644); err != nil {
			return nil, nil, fmt.Errorf("写入默认参数文件失败: %w", err)
		}
		notes = append(notes, fmt.Sprintf("参数目录为空，已生成默认参数文件: %s", path))
	} else if err != nil {
		return nil, nil, fmt.Errorf("检查参数文件失败: %w", err)
	}

	s := &Store{path: path, cfg: Default()}
	// Start the version marker empty so a file written before it existed reads
	// back as version 0 and gets migrated; pre-filling it from Default() would
	// hide the very thing the marker is there to detect.
	s.cfg.ConfigVersion = 0
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, notes, fmt.Errorf("读取参数文件失败: %w", err)
	}
	if err := yaml.Unmarshal(raw, &s.cfg); err != nil {
		// A malformed file must not brick the app: fall back to defaults and
		// tell the user which file is at fault.
		notes = append(notes, fmt.Sprintf("参数文件解析失败(%v)，已回退到默认参数: %s", err, path))
		s.cfg = Default()
	}
	if note, changed := migrateConfig(&s.cfg); changed {
		notes = append(notes, note)
		if data, err := yaml.Marshal(s.cfg); err == nil {
			if err := os.WriteFile(path, data, 0o644); err != nil {
				notes = append(notes, fmt.Sprintf("保存升级后的参数失败: %v", err))
			}
		}
	}
	notes = append(notes, s.cfg.Validate()...)
	return s, notes, nil
}

// migrateConfig brings a settings file written by an earlier release up to the
// current defaults, once. A file that already carries the current version is
// left exactly as the user saved it, so a deliberate choice of 原文件格式 is
// never silently reverted.
func migrateConfig(c *Config) (string, bool) {
	if c.ConfigVersion >= currentConfigVersion {
		return "", false
	}
	note := ""
	if c.ConfigVersion < 2 && c.ExportMode == ExportTemplate {
		c.ExportMode = ExportClean
		note = "导出方式默认值已改为「纯数据」，可在参数设定中改回「原文件格式」"
	}
	c.ConfigVersion = currentConfigVersion
	return note, true
}

// Get returns a copy of the active parameters.
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Save validates, persists and activates the supplied parameters. It returns
// any corrections that were applied.
func (s *Store) Save(c Config) ([]string, error) {
	notes := c.Validate()
	data, err := yaml.Marshal(c)
	if err != nil {
		return notes, fmt.Errorf("序列化参数失败: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return notes, fmt.Errorf("保存参数文件失败: %w", err)
	}
	s.cfg = c
	return notes, nil
}

// Path is the absolute location of the active parameter file.
func (s *Store) Path() string { return s.path }

// ResetToDefault rewrites the parameter file from the embedded default.
func (s *Store) ResetToDefault() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.WriteFile(s.path, defaultYAML, 0o644); err != nil {
		return err
	}
	s.cfg = Default()
	return nil
}

// configDir picks a per-user location, preferring the directory next to the
// executable so a portable install keeps its settings alongside the binary.
func configDir() (string, error) { return stateDir("config") }

// ProgramDir is the folder the application treats as its own: where the
// executable lives, or — for a macOS bundle — the folder holding CLEAR.app, so
// the data and config folders sit next to what the user double-clicked instead
// of inside the package.
//
// A build produced by `go run` or `wails dev` lands in a throwaway folder, so
// that case falls back to the working directory: anchoring state in a temp
// folder would hand every run an empty database.
func ProgramDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("无法确定程序位置: %w", err)
	}
	dir := programDirFrom(exe)
	if isTempDir(dir) {
		if wd, err := os.Getwd(); err == nil && isWritable(wd) {
			return wd, nil
		}
	}
	return dir, nil
}

// programDirFrom resolves the folder holding the runnable from its path.
func programDirFrom(exe string) string {
	dir := filepath.Dir(exe)
	if filepath.Base(dir) != "MacOS" {
		return dir
	}
	contents := filepath.Dir(dir)
	if filepath.Base(contents) != "Contents" {
		return dir
	}
	bundle := filepath.Dir(contents)
	if !strings.HasSuffix(filepath.Base(bundle), ".app") {
		return dir
	}
	return filepath.Dir(bundle)
}

func isTempDir(dir string) bool { return isTempDirIn(dir, os.TempDir()) }

func isTempDirIn(dir, tempRoot string) bool {
	if tempRoot == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(tempRoot), filepath.Clean(dir))
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// stateDir prefers <program dir>/<name> so a portable install keeps everything
// beside the executable, and falls back to ~/.clear/<name> when the program
// folder is read-only — an app in /Applications, a quarantined bundle, a
// package manager's read-only prefix.
func stateDir(name string) (string, error) {
	if base, err := ProgramDir(); err == nil {
		dir := filepath.Join(base, name)
		if isWritable(dir) {
			return dir, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法确定用户目录: %w", err)
	}
	return filepath.Join(home, ".clear", name), nil
}

// LegacyBases lists the folders an older CLEAR may have written its data and
// config folders into, so they can be adopted instead of silently abandoned.
func LegacyBases() []string {
	var out []string
	if exe, err := os.Executable(); err == nil {
		out = append(out, filepath.Dir(exe))
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, ".clear"))
	}
	return out
}

// AdoptDir copies a folder written by an older CLEAR into the current location
// the first time it is needed, when the current location has nothing of its
// own yet. It copies rather than moves, so the original survives untouched and
// nothing is lost if the new location is not the one that ends up being used.
//
// It returns the folder it copied from, or "" when there was nothing to do.
func AdoptDir(name, marker, current string, bases []string) (string, error) {
	if fileExists(filepath.Join(current, marker)) {
		return "", nil
	}
	for _, base := range bases {
		old := filepath.Join(base, name)
		if old == current || !fileExists(filepath.Join(old, marker)) {
			continue
		}
		if err := copyTree(old, current); err != nil {
			return "", err
		}
		return old, nil
	}
	return "", nil
}

// AdoptDataDir adopts a database left in an older location and reports where it
// came from, so startup can say so in the log.
func AdoptDataDir(dir string) string {
	from, err := AdoptDir("data", "clear.db", dir, LegacyBases())
	if err != nil {
		return ""
	}
	return from
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil // skip sockets, devices, dangling links
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func isWritable(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	probe := filepath.Join(dir, ".write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return false
	}
	_ = os.Remove(probe)
	return true
}

// DataDir holds the database, stored templates and logs.
func DataDir() (string, error) {
	dir, err := stateDir("data")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

var _ = runtime.GOOS
