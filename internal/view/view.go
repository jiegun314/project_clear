// Package view holds the payloads the frontend receives.
//
// They live outside package main so the binding surface can be tested: every one
// of these crosses Wails' JSON round trip, and a value that does not survive it
// (a nil slice becoming null) otherwise fails only inside the webview.
package view

import (
	"project_clear/internal/config"
	"project_clear/internal/mps"
	"project_clear/internal/service"
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

// Status backs the status bar and the workspace strip: the draft, the two
// completed inputs, and the week's integration (including whether a newer
// import has made it expired).
type Status struct {
	HasDraft   bool   `json:"hasDraft"`
	DraftID    int64  `json:"draftId"`
	DraftWeek  string `json:"draftWeek"`
	DraftRows  int    `json:"draftRows"`
	DraftFiles int    `json:"draftFiles"`
	DraftFail  int    `json:"draftFailed"`

	HasGlobal     bool   `json:"hasGlobal"`
	GlobalID      int64  `json:"globalId"`
	GlobalWeek    string `json:"globalWeek"`
	GlobalRows    int    `json:"globalRows"`
	GlobalFiles   int    `json:"globalFiles"`
	GlobalVersion int    `json:"globalVersion"`
	GlobalAt      string `json:"globalAt"`
	GlobalWeeks   int    `json:"globalWeeks"`

	HasLocal     bool   `json:"hasLocal"`
	LocalID      int64  `json:"localId"`
	LocalWeek    string `json:"localWeek"`
	LocalRows    int    `json:"localRows"`
	LocalFiles   int    `json:"localFiles"`
	LocalVersion int    `json:"localVersion"`
	LocalAt      string `json:"localAt"`
	LocalWeeks   int    `json:"localWeeks"`
	// LocalFileName is the single Local workbook, shown in the status strip:
	// Local has no per-file list because one import is the whole dataset.
	LocalFileName string `json:"localFileName"`

	HasIntegration     bool   `json:"hasIntegration"`
	IntegrationID      int64  `json:"integrationId"`
	IntegrationWeek    string `json:"integrationWeek"`
	IntegrationRows    int    `json:"integrationRows"`
	IntegrationAt      string `json:"integrationAt"`
	IntegrationExpired bool   `json:"integrationExpired"`

	ActiveWeek   string `json:"activeWeek"`
	Unintegrated bool   `json:"unintegrated"`
	// CanDiscardWeek is true when the week has imports the integration does not
	// include, so "丢弃本周导入" has something to remove.
	CanDiscardWeek bool `json:"canDiscardWeek"`

	ArchivedWeeks int `json:"archivedWeeks"`
	ArchivedRows  int `json:"archivedRows"`

	ReadColumns     int    `json:"readColumns"`
	GlobalLOCFilter string `json:"globalLocFilter"`
	LocalLOCFilter  string `json:"localLocFilter"`
	PageSize        int    `json:"pageSize"`
	HeaderDisplay   string `json:"headerDisplay"`
	ExportMode      string `json:"exportMode"`
	Database        string `json:"database"`
	LastAction      string `json:"lastAction"`
}

// CommitResult reports the outcome of 整合.
type CommitResult struct {
	WeekCode    string `json:"weekCode"`
	WeekStart   string `json:"weekStart"`
	MergedID    int64  `json:"mergedId"`
	GlobalID    int64  `json:"globalId"`
	LocalID     int64  `json:"localId"`
	MergedRows  int    `json:"mergedRows"`
	GlobalRows  int    `json:"globalRows"`
	LocalRows   int    `json:"localRows"`
	GlobalVer   int    `json:"globalVersion"`
	LocalVer    int    `json:"localVersion"`
	MergedVer   int    `json:"mergedVersion"`
	FileCount   int    `json:"fileCount"`
	CommittedAt string `json:"committedAt"`
	Overwrote   bool   `json:"overwrote"`
	WasExpired  bool   `json:"wasExpired"`
	// RequestedWeeks is the selected cutoff (26 = AO, 61 = BX); MergedWeeks is
	// what survived the Global/Local intersection.
	RequestedWeeks int      `json:"requestedWeeks"`
	MergedWeeks    int      `json:"mergedWeeks"`
	Warnings       []string `json:"warnings"`
}

// DiscardWeekResult reports 丢弃本周导入.
type DiscardWeekResult struct {
	WeekCode string `json:"weekCode"`
	Datasets int    `json:"datasets"`
}

// FilesView backs the 已导入文件 window: the workbooks of the current draft or
// completed Global plus the current Local.
type FilesView struct {
	Files []service.FileWithDataset `json:"files"`
	// HasDraft says the Global side shown here is still being assembled.
	HasDraft bool `json:"hasDraft"`
	// FileCount/RowCount/FailedCount are the dataset totals. They matter for a
	// week migrated from v4: that release kept the counts but not the per-file
	// rows, so the table is empty while the totals are not.
	FileCount   int    `json:"fileCount"`
	RowCount    int    `json:"rowCount"`
	FailedCount int    `json:"failedCount"`
	Note        string `json:"note,omitempty"`
}

// ExportResult reports the outcome of 导出.
type ExportResult struct {
	DestPath     string `json:"destPath"`
	DatasetID    int64  `json:"datasetId"`
	WeekCode     string `json:"weekCode"`
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
	Sheets       int    `json:"sheets"`
	// Expired says a re-import has superseded the integration this file came from.
	Expired bool `json:"expired"`
}

// GridQuery is the data-grid request.
type GridQuery struct {
	// DatasetID is which dataset to read: a draft, a completed Global/Local, or
	// the merged result of an integration.
	DatasetID int64             `json:"datasetId"`
	Page      int               `json:"page"`
	PageSize  int               `json:"pageSize"`
	Search    string            `json:"search"`
	SortField string            `json:"sortField"`
	SortDesc  bool              `json:"sortDesc"`
	Filters   map[string]string `json:"filters"`
}

// GridHeader describes the columns the grid must render.
type GridHeader struct {
	DatasetID  int64      `json:"datasetId"`
	Kind       string     `json:"kind"`
	Version    int        `json:"version"`
	WeekCode   string     `json:"weekCode"`
	WeekStart  string     `json:"weekStart"`
	IndexNames []string   `json:"indexNames"`
	Weeks      []mps.Week `json:"weeks"`
	Total      int        `json:"total"`
	Expired    bool       `json:"expired"`
}

// ConfigView is the settings payload.
type ConfigView struct {
	Config config.Config `json:"config"`
	Path   string        `json:"path"`
}

// NewGridHeader returns a header with its list fields initialised.
func NewGridHeader() *GridHeader {
	return &GridHeader{IndexNames: []string{}, Weeks: []mps.Week{}}
}

// NewFilesView returns a file list that is empty, not null.
func NewFilesView() *FilesView {
	return &FilesView{Files: []service.FileWithDataset{}}
}

// NonNil replaces every nil slice on the payload with an empty one.
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
func (v *FilesView) NonNil() *FilesView {
	if v.Files == nil {
		v.Files = []service.FileWithDataset{}
	}
	return v
}
