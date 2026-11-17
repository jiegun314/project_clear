package service

import (
	"fmt"
	"strings"
	"time"

	"project_clear/internal/store"
)

// dataGroupRows is the complete data group for one ITEM and one warehouse:
// the 14 element rows A..O (AdjDmd through BackOrder $).
const dataGroupRows = 14

// cleanupStats reports what a data cleanup changed. It is counted in rows and
// in file-level groups so the import log can explain both a dropped duplicate
// and an incomplete group.
type cleanupStats struct {
	InputRows        int
	KeptRows         int
	DroppedRows      int
	IncompleteGroups int
	DuplicateGroups  int
}

func (s cleanupStats) summary() string {
	return fmt.Sprintf("数据清理：%d 行 → %d 行，删除 %d 行（不完整组 %d 个，重复完整组 %d 个）",
		s.InputRows, s.KeptRows, s.DroppedRows, s.IncompleteGroups, s.DuplicateGroups)
}

// cleanImportRows keeps exactly one complete 14-row group for every
// ITEM + first LOC (warehouse) pair.
//
// A file's rows for one pair form a complete group when they contain all 14
// distinct O values. The first occurrence of each value is the group; extra
// duplicate rows in the same file are discarded. If several files have a
// complete group, the one whose source file was created latest wins. A pair
// with no complete group is removed entirely, including its partial rows.
//
// The returned slice preserves the input order. createdAt resolves a row's
// source file to its creation time; a zero time is treated as "unknown" and
// loses a comparison against a known time.
func cleanImportRows(rows []store.StagedRow, createdAt func(file string) time.Time) ([]store.StagedRow, cleanupStats) {
	stats := cleanupStats{InputRows: len(rows)}
	if createdAt == nil {
		createdAt = func(string) time.Time { return time.Time{} }
	}

	type fileGroup struct {
		key     string
		indices []int
		rows    []int // first row for each distinct O value
		created time.Time
	}
	type keyGroup struct {
		order []string
		files map[string]*fileGroup
	}

	groups := map[string]*keyGroup{}
	groupOrder := []string{}
	keep := make([]bool, len(rows))

	for i := range rows {
		item := strings.TrimSpace(rows[i].Index[8])
		warehouse := strings.TrimSpace(rows[i].Index[11])
		if item == "" || warehouse == "" {
			stats.IncompleteGroups++
			continue
		}
		key := item + "\x00" + warehouse
		g := groups[key]
		if g == nil {
			g = &keyGroup{files: map[string]*fileGroup{}}
			groups[key] = g
			groupOrder = append(groupOrder, key)
		}
		file := strings.TrimSpace(rows[i].FilePath)
		if file == "" {
			file = strings.TrimSpace(rows[i].FileName)
		}
		fg := g.files[file]
		if fg == nil {
			fg = &fileGroup{key: file, created: createdAt(file)}
			g.files[file] = fg
			g.order = append(g.order, file)
		}
		fg.indices = append(fg.indices, i)
	}

	for _, key := range groupOrder {
		g := groups[key]
		var candidates []*fileGroup
		for _, file := range g.order {
			fg := g.files[file]
			first := map[string]int{}
			picked := make([]int, 0, dataGroupRows)
			for _, idx := range fg.indices {
				field := strings.TrimSpace(rows[idx].Index[14])
				if field == "" {
					continue
				}
				if _, ok := first[field]; ok {
					continue
				}
				first[field] = idx
				picked = append(picked, idx)
			}
			if len(first) != dataGroupRows {
				stats.IncompleteGroups++
				continue
			}
			fg.rows = picked
			candidates = append(candidates, fg)
		}
		if len(candidates) == 0 {
			continue
		}
		best := candidates[0]
		for _, c := range candidates[1:] {
			if c.created.After(best.created) {
				best = c
			}
		}
		for _, idx := range best.rows {
			keep[idx] = true
		}
		if len(candidates) > 1 {
			stats.DuplicateGroups += len(candidates) - 1
		}
	}

	out := make([]store.StagedRow, 0, len(rows))
	for i, row := range rows {
		if keep[i] {
			out = append(out, row)
			continue
		}
		stats.DroppedRows++
	}
	stats.KeptRows = len(out)
	return out, stats
}

// cleanDraftLocked applies cleanImportRows to every row currently in a Global
// draft. It is called after the draft has been written, so an add sees both the
// copied base rows and the new files.
func (s *Service) cleanDraftLocked(datasetID int64) (cleanupStats, error) {
	rows, err := s.db.DatasetRows(datasetID)
	if err != nil {
		return cleanupStats{}, err
	}
	files, err := s.db.DatasetFiles(datasetID)
	if err != nil {
		return cleanupStats{}, err
	}
	byName := make(map[string]string, len(files))
	for _, f := range files {
		byName[f.Name] = f.Path
	}
	resolve := func(file string) time.Time {
		if path, ok := byName[file]; ok {
			file = path
		}
		return fileCreatedAt(file)
	}
	kept, stats := cleanImportRows(rows, resolve)
	if stats.DroppedRows == 0 {
		return stats, nil
	}
	keepSeq := make(map[int]bool, len(kept))
	for _, r := range kept {
		keepSeq[r.Seq] = true
	}
	drop := make([]int, 0, stats.DroppedRows)
	for _, r := range rows {
		if !keepSeq[r.Seq] {
			drop = append(drop, r.Seq)
		}
	}
	if err := s.db.DeleteDatasetRows(datasetID, drop); err != nil {
		return stats, err
	}
	return stats, nil
}
