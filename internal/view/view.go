// Package view holds the payloads the frontend receives.
//
// They live outside package main so the binding surface can be tested: every one
// of these crosses Wails' JSON round trip, and a value that does not survive it
// (a nil slice becoming null, a time.Time becoming an opaque object) otherwise
// fails only inside the webview, at render time.
package view

import (
	"project_clear/internal/config"
	"project_clear/internal/mps"
	"project_clear/internal/store"
)

// AppInfo describes the application for the About window.
type AppInfo struct {
	Name       string `json:"name"`
	FullName   string `json:"fullName"`
	Version    string `json:"version"`
	DataDir    string `json:"dataDir"`
	ConfigPath string `json:"configPath"`
	Database   string `json:"database"`
	GoVersion  string `json:"goVersion"`
	Platform   string `json:"platform"`
}

// Status backs the status bar.
type Status struct {
	HasStaging    bool   `json:"hasStaging"`
	WeekCode      string `json:"weekCode"`
	WeekStart     string `json:"weekStart"`
	StagedRows    int    `json:"stagedRows"`
	ArchivedWeeks int    `json:"archivedWeeks"`
	ArchivedRows  int    `json:"archivedRows"`
	ReadColumns   int    `json:"readColumns"`
	LOCFilter     string `json:"locFilter"`
	PageSize      int    `json:"pageSize"`
	HeaderDisplay string `json:"headerDisplay"`
	ExportMode    string `json:"exportMode"`
	Database      string `json:"database"`
	LastAction    string `json:"lastAction"`
}

// CommitResult reports the outcome of 整合.
type CommitResult struct {
	WeekCode    string `json:"weekCode"`
	WeekStart   string `json:"weekStart"`
	TableName   string `json:"tableName"`
	RowCount    int    `json:"rowCount"`
	FileCount   int    `json:"fileCount"`
	CommittedAt string `json:"committedAt"`
	Overwrote   bool   `json:"overwrote"`
}

// StagingFilesView backs the toolbar's 已导入文件 button: which workbooks are in
// the integration list right now and how many rows they brought in. 清空 leaves
// the list empty again.
type StagingFilesView struct {
	HasStaging bool `json:"hasStaging"`
	FileCount  int  `json:"fileCount"`
	RowCount   int  `json:"rowCount"`
	Failed     int  `json:"failedCount"`
	// BatchState is "staging" while the list is still waiting for 整合 and
	// "committed" when it is the record of the batch that was just integrated.
	BatchState string                   `json:"batchState"`
	Files      []store.StagedFileDetail `json:"files"`
}

// ClearStagingResult reports what 清空 removed.
type ClearStagingResult struct {
	WeekCode string `json:"weekCode"`
	Rows     int    `json:"rows"`
}

// ExportResult reports the outcome of 导出.
type ExportResult struct {
	DestPath     string `json:"destPath"`
	Mode         string `json:"mode"`
	Rows         int    `json:"rows"`
	Cols         int    `json:"cols"`
	Comments     int    `json:"comments"`
	CFRows       int    `json:"cfRows"`
	StylesUsed   int    `json:"stylesUsed"`
	Dropped      int    `json:"dropped"`
	SizeBytes    int64  `json:"sizeBytes"`
	PreservedVBA bool   `json:"preservedVba"`
	DurationMS   int64  `json:"durationMs"`
}

// GridQuery is the data-grid request.
type GridQuery struct {
	// Source is "" for the staging area or a week code such as "2639".
	Source    string            `json:"source"`
	Page      int               `json:"page"`
	PageSize  int               `json:"pageSize"`
	Search    string            `json:"search"`
	SortField string            `json:"sortField"`
	SortDesc  bool              `json:"sortDesc"`
	Filters   map[string]string `json:"filters"`
}

// GridHeader describes the columns the grid must render.
type GridHeader struct {
	Source     string     `json:"source"`
	WeekCode   string     `json:"weekCode"`
	WeekStart  string     `json:"weekStart"`
	IndexNames []string   `json:"indexNames"`
	Weeks      []mps.Week `json:"weeks"`
	Total      int        `json:"total"`
	HasStaging bool       `json:"hasStaging"`
}

// ConfigView is the settings payload.
type ConfigView struct {
	Config config.Config `json:"config"`
	Path   string        `json:"path"`
}

// The list fields below are created empty rather than left nil.
//
// Wails marshals every return value with encoding/json, which turns a nil slice
// into null — and the first render walked those arrays, so an empty header that
// arrived as null took the window down with a blank page. Seeding them here
// rather than at each call site is what makes the rule checkable from a test.

// NewGridHeader returns a header with its list fields initialised.
func NewGridHeader(source string) *GridHeader {
	return &GridHeader{Source: source, IndexNames: []string{}, Weeks: []mps.Week{}}
}

// NewStagingFilesView returns a file list that is empty, not null.
func NewStagingFilesView() *StagingFilesView {
	return &StagingFilesView{Files: []store.StagedFileDetail{}}
}

// NonNil replaces every nil slice on the payload with an empty one. It is the
// belt-and-braces path for the values built from stored data, where a nil slice
// is otherwise easy to hand back by accident.
func (h *GridHeader) NonNil() *GridHeader {
	if h.IndexNames == nil {
		h.IndexNames = []string{}
	}
	if h.Weeks == nil {
		h.Weeks = []mps.Week{}
	}
	return h
}

// NonNil replaces a nil file list with an empty one.
func (v *StagingFilesView) NonNil() *StagingFilesView {
	if v.Files == nil {
		v.Files = []store.StagedFileDetail{}
	}
	return v
}
