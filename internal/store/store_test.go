package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jianbao233/foxhole-quartermaster/internal/ingest"
	"github.com/Jianbao233/foxhole-quartermaster/internal/store"
)

const ownerID = "76561198000000001"

func newStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := st.Seed(); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ownerID); err != nil {
		t.Fatal(err)
	}
	return st
}

func loadSample(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "parser", "testdata", "sample_haibour_v1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestIngestEndToEnd(t *testing.T) {
	st := newStore(t)
	raw := loadSample(t)

	// 第一次 ingest（新别名）
	res, err := ingest.Ingest(context.Background(), st, raw, ownerID, "东门仓", true)
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if res.Duplicate {
		t.Fatal("first should not be duplicate")
	}
	if res.ItemCount < 200 {
		t.Errorf("item count too small: %d", res.ItemCount)
	}

	// 同一份内容再来一次 → dedup
	res2, err := ingest.Ingest(context.Background(), st, raw, ownerID, "东门仓", false)
	if err != nil {
		t.Fatalf("dup ingest: %v", err)
	}
	if !res2.Duplicate {
		t.Fatal("second should be duplicate")
	}
	if res2.SnapshotID != res.SnapshotID {
		t.Errorf("dup should return same snap id: %d vs %d", res2.SnapshotID, res.SnapshotID)
	}

	// 验证 Stockpile / Snapshot 列表能查到
	wars, err := st.ListWars()
	if err != nil || len(wars) < 1 {
		t.Fatalf("wars: %v %v", err, wars)
	}
	sps, err := st.ListStockpilesByWar(res.WarID)
	if err != nil || len(sps) != 1 {
		t.Fatalf("expect 1 stockpile, got %d err=%v", len(sps), err)
	}
	if sps[0].PlayerAlias != "东门仓" {
		t.Errorf("alias=%q", sps[0].PlayerAlias)
	}

	items, capturedAt, err := st.GetLatestItemsByStockpile(res.StockpileID)
	if err != nil {
		t.Fatal(err)
	}
	if capturedAt == 0 || len(items) < 100 {
		t.Errorf("latest items count=%d capturedAt=%d", len(items), capturedAt)
	}

	// 验证 RecentSnapshots
	uploads, err := st.RecentSnapshots(10)
	if err != nil || len(uploads) != 1 {
		t.Fatalf("recent uploads expect 1, got %d err=%v", len(uploads), err)
	}
}

func TestIngestNewAliasRequired(t *testing.T) {
	st := newStore(t)
	raw := loadSample(t)
	_, err := ingest.Ingest(context.Background(), st, raw, ownerID, "新名字", false)
	if err == nil {
		t.Fatal("expected new_alias_required error")
	}
}

func TestIngestUnknownItem(t *testing.T) {
	st := newStore(t)
	// 删一个 alias，模拟未知物品
	if _, err := st.DB.Exec(`DELETE FROM item_aliases WHERE alias='基础材料'`); err != nil {
		t.Fatal(err)
	}
	raw := loadSample(t)
	_, err := ingest.Ingest(context.Background(), st, raw, ownerID, "东门仓", true)
	if err == nil {
		t.Fatal("expected unknown_item error")
	}
	if _, ok := err.(*ingest.ErrUnknownItem); !ok {
		t.Fatalf("err type wrong: %T %v", err, err)
	}
}

func TestStockpileUniquenessAndUpdate(t *testing.T) {
	st := newStore(t)
	raw := loadSample(t)
	res1, err := ingest.Ingest(context.Background(), st, raw, ownerID, "alias1", true)
	if err != nil {
		t.Fatal(err)
	}
	// 改一处数量后再 ingest，应该是同一个 stockpile，新 snapshot
	raw2 := raw
	// 把 "基础材料 (箱装),260" 改成 261
	raw2 = replaceFirst(raw2, "基础材料 (箱装),260", "基础材料 (箱装),261")
	res2, err := ingest.Ingest(context.Background(), st, raw2, ownerID, "alias1", false)
	if err != nil {
		t.Fatal(err)
	}
	if res2.StockpileID != res1.StockpileID {
		t.Errorf("expect same stockpile_id, got %d vs %d", res2.StockpileID, res1.StockpileID)
	}
	if res2.SnapshotID == res1.SnapshotID {
		t.Errorf("expect new snap, got same %d", res2.SnapshotID)
	}
	if res2.Duplicate {
		t.Errorf("modified content should not be duplicate")
	}
}

func replaceFirst(s, old, new string) string {
	idx := indexOf(s, old)
	if idx < 0 {
		return s
	}
	return s[:idx] + new + s[idx+len(old):]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}