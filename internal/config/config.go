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
	// ExportTemplate rewrites a stored copy of the first source workbook, so
	// every visual detail of the original survives.
	ExportTemplate ExportMode = "template"
	// ExportClean rebuilds a fresh workbook containing only the MPS sheet.
	ExportClean ExportMode = "clean"
)

// Config is the full parameter set exposed on the settings screen.
type Config struct {
	// ReadColumns is how many week columns to read starting at column P.
	ReadColumns int `yaml:"readColumns" json:"readColumns"`
	// LOCFilter is the value required in the LOC column (column L) for a row
	// to be kept.
	LOCFilter string `yaml:"locFilter" json:"locFilter"`
	// PageSize is the row count per page in the main data grid.
	PageSize int `yaml:"pageSize" json:"pageSize"`
	// HeaderDisplay is twoRow (default) or oneRow.
	HeaderDisplay HeaderDisplay `yaml:"headerDisplay" json:"headerDisplay"`
	// ExportMode is template (default) or clean.
	ExportMode ExportMode `yaml:"exportMode" json:"exportMode"`
	// ExportDir remembers the last used export directory.
	ExportDir string `yaml:"exportDir" json:"exportDir"`
}

// Default returns the built-in parameter set, matching the specification.
func Default() Config {
	return Config{
		ReadColumns:   20,
		LOCFilter:     "WH_CNB",
		PageSize:      200,
		HeaderDisplay: HeaderTwoRow,
		ExportMode:    ExportTemplate,
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
		notes = append(notes, fmt.Sprintf("导出模式 %q 无效，已重置为 template", c.ExportMode))
		c.ExportMode = ExportTemplate
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("创建参数目录失败: %w", err)
	}
	path := filepath.Join(dir, "clear.yaml")
	var notes []string
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, defaultYAML, 0o644); err != nil {
			return nil, nil, fmt.Errorf("写入默认参数文件失败: %w", err)
		}
		notes = append(notes, fmt.Sprintf("参数目录为空，已生成默认参数文件: %s", path))
	} else if err != nil {
		return nil, nil, fmt.Errorf("检查参数文件失败: %w", err)
	}

	s := &Store{path: path, cfg: Default()}
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
	notes = append(notes, s.cfg.Validate()...)
	return s, notes, nil
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
func configDir() (string, error) {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "config")
		if isWritable(dir) {
			return dir, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法确定用户目录: %w", err)
	}
	dir := filepath.Join(home, ".clear")
	return filepath.Join(dir, "config"), nil
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
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "data")
		if isWritable(dir) {
			return dir, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法确定用户目录: %w", err)
	}
	dir := filepath.Join(home, ".clear", "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

var _ = runtime.GOOS
