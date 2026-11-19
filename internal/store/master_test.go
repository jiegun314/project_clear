package store

import (
	"path/filepath"
	"testing"

	"project_clear/internal/master"
)

func openMasterStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "clear.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func testMasterWorkbook() *master.Workbook {
	return &master.Workbook{
		Name: "match.xlsx",
		Items: []master.ItemEntry{
			{ItemCode: "I1", BaseCode: "B1"},
			{ItemCode: "I2", BaseCode: "B2"},
			{ItemCode: "I3", BaseCode: "MISSING"},
		},
		Bases: []master.BaseEntry{
			{BaseCode: "B1", ManuCode: "M1", SapCode: "S1"},
			{BaseCode: "B2", ManuCode: "M2", SapCode: "S2"},
		},
		Categories: []master.CategoryEntry{{SapCode: "S1", Category: "Cat A"}},
	}
}

func TestReplaceMasterDataOverviewAndPages(t *testing.T) {
	st := openMasterStore(t)
	if err := st.ReplaceMasterData(testMasterWorkbook()); err != nil {
		t.Fatal(err)
	}
	ov, err := st.MasterOverview()
	if err != nil {
		t.Fatal(err)
	}
	if ov.ItemCount != 3 || ov.BaseCount != 2 || ov.SapCount != 2 || ov.CategoryCount != 1 {
		t.Fatalf("overview = %+v", ov)
	}
	if ov.Uncategorized != 1 || ov.OrphanCount != 1 || ov.UnusedCategories != 0 {
		t.Fatalf("overview flags = %+v", ov)
	}
	if ov.LastImportAt == "" || len(ov.Categories) != 1 || ov.Categories[0] != "Cat A" {
		t.Fatalf("overview import/categories = %+v", ov)
	}

	orphans, err := st.MasterPage(master.Query{Table: master.TableItem, Page: 1, PageSize: 50, Filter: "orphan"})
	if err != nil {
		t.Fatal(err)
	}
	if orphans.Total != 1 || len(orphans.Rows) != 1 || orphans.Rows[0].ItemCode != "I3" || !orphans.Rows[0].Orphan {
		t.Fatalf("orphans = %+v", orphans)
	}

	uncat, err := st.MasterPage(master.Query{Table: master.TableBase, Page: 1, PageSize: 50, Filter: "uncategorized"})
	if err != nil {
		t.Fatal(err)
	}
	if uncat.Total != 1 || uncat.Rows[0].BaseCode != "B2" || !uncat.Rows[0].Uncategorized {
		t.Fatalf("uncategorized = %+v", uncat)
	}
	if uncat.Rows[0].RefCount != 1 {
		t.Fatalf("B2 refCount = %d, want 1", uncat.Rows[0].RefCount)
	}

	cat, err := st.MasterPage(master.Query{Table: master.TableCategory, Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if cat.Total != 1 || cat.Rows[0].RefCount != 1 || cat.Rows[0].Unused {
		t.Fatalf("categories = %+v", cat)
	}

	search, err := st.MasterPage(master.Query{Table: master.TableBase, Page: 1, PageSize: 50, Search: "M2"})
	if err != nil {
		t.Fatal(err)
	}
	if search.Total != 1 || search.Rows[0].BaseCode != "B2" {
		t.Fatalf("search = %+v", search)
	}
}

func TestSaveAndDeleteMasterRows(t *testing.T) {
	st := openMasterStore(t)
	if err := st.ReplaceMasterData(testMasterWorkbook()); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveMasterRow(master.Input{
		Table: master.TableCategory, SapCode: "S2", Category: "Cat B",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveMasterRow(master.Input{
		Table: master.TableItem, ItemCode: "I4", BaseCode: "B1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveMasterRow(master.Input{
		Table: master.TableBase, BaseCode: "B3", ManuCode: "M3", SapCode: "S2",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteMasterRow(master.TableItem, "I4"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteMasterRow(master.TableBase, "B1"); err == nil {
		t.Fatal("expected Base B1 delete to be refused while an Item references it")
	}
	if err := st.DeleteMasterRow(master.TableCategory, "S1"); err == nil {
		t.Fatal("expected SAP S1 delete to be refused while a Base references it")
	}
	if err := st.DeleteMasterRow(master.TableBase, "B3"); err != nil {
		t.Fatal(err)
	}

	ov, err := st.MasterOverview()
	if err != nil {
		t.Fatal(err)
	}
	if ov.CategoryCount != 2 {
		t.Fatalf("category count = %d, want 2", ov.CategoryCount)
	}
	var audits int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM master_audit`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits < 5 {
		t.Fatalf("audit rows = %d, want >= 5", audits)
	}
}

func TestSaveMasterRowEditsInPlace(t *testing.T) {
	st := openMasterStore(t)
	if err := st.ReplaceMasterData(testMasterWorkbook()); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveMasterRow(master.Input{
		Table: master.TableItem, OriginalKey: "I1", ItemCode: "I1", BaseCode: "B2",
	}); err != nil {
		t.Fatal(err)
	}
	page, err := st.MasterPage(master.Query{Table: master.TableItem, Page: 1, PageSize: 50, Search: "I1"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Rows[0].BaseCode != "B2" {
		t.Fatalf("updated item = %+v", page.Rows)
	}
}

func priceCost(v float64) *float64 { return &v }

func TestReplacePriceDataOverviewAndPages(t *testing.T) {
	st := openMasterStore(t)
	w := &master.PriceWorkbook{
		Name: "price.xlsx",
		Prices: []master.PriceEntry{
			{Material: "000.009", StandardCost: priceCost(107.36)},
			{Material: "000.010"},
			{Material: "000.011", StandardCost: priceCost(0)},
		},
		MissingCosts: 1,
	}
	if err := st.ReplacePriceData(w); err != nil {
		t.Fatal(err)
	}
	ov, err := st.MasterOverview()
	if err != nil {
		t.Fatal(err)
	}
	if ov.PriceCount != 3 || ov.PriceMissing != 1 || ov.LastPriceImportAt == "" {
		t.Fatalf("overview = %+v", ov)
	}
	if ov.LastImportAt != "" {
		t.Fatalf("code import timestamp = %q, want empty", ov.LastImportAt)
	}

	all, err := st.MasterPage(master.Query{Table: master.TablePrice, Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 3 || len(all.Rows) != 3 || all.Rows[0].Material != "000.009" || all.Rows[2].StandardCost == nil {
		t.Fatalf("price page = %+v", all)
	}

	missing, err := st.MasterPage(master.Query{Table: master.TablePrice, Page: 1, PageSize: 50, Filter: "unpriced"})
	if err != nil {
		t.Fatal(err)
	}
	if missing.Total != 1 || missing.Rows[0].Material != "000.010" || missing.Rows[0].StandardCost != nil {
		t.Fatalf("unpriced page = %+v", missing)
	}

	search, err := st.MasterPage(master.Query{Table: master.TablePrice, Page: 1, PageSize: 50, Search: "011"})
	if err != nil {
		t.Fatal(err)
	}
	if search.Total != 1 || search.Rows[0].Material != "000.011" {
		t.Fatalf("price search = %+v", search)
	}
}

func TestSaveAndDeletePriceRows(t *testing.T) {
	st := openMasterStore(t)
	if err := st.SaveMasterRow(master.Input{
		Table: master.TablePrice, Material: "000.009", StandardCost: priceCost(100),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveMasterRow(master.Input{
		Table: master.TablePrice, OriginalKey: "000.009", Material: "000.009", StandardCost: priceCost(120),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveMasterRow(master.Input{
		Table: master.TablePrice, Material: "000.010",
	}); err != nil {
		t.Fatal(err)
	}
	ov, err := st.MasterOverview()
	if err != nil {
		t.Fatal(err)
	}
	if ov.PriceCount != 2 || ov.PriceMissing != 1 {
		t.Fatalf("overview = %+v", ov)
	}
	if err := st.DeleteMasterRow(master.TablePrice, "000.010"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteMasterRow(master.TablePrice, "000.009"); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveMasterRow(master.Input{Table: master.TablePrice}); err == nil {
		t.Fatal("expected blank Material to be rejected")
	}
	var audits int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM master_audit WHERE table_name='price'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 5 {
		t.Fatalf("price audits = %d, want 5", audits)
	}
}
