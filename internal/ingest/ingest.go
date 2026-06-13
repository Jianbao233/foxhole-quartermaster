// Package ingest 串联：解析 → 物品 alias 校验 → upsert stockpile → 写 snapshot。
package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Jianbao233/foxhole-quartermaster/internal/parser"
	"github.com/Jianbao233/foxhole-quartermaster/internal/store"
)

// Result 是 /api/ingest 的成功返回。
type Result struct {
	StockpileID int64  `json:"stockpile_id"`
	SnapshotID  int64  `json:"snapshot_id"`
	WarID       int64  `json:"war_id"`
	ItemCount   int    `json:"item_count"`
	Duplicate   bool   `json:"duplicate"`
	CapturedAt  int64  `json:"captured_at"`
	Region      string `json:"region"`
	SubRegion   string `json:"sub_region"`
	Building    string `json:"building"`
	Visibility  string `json:"visibility"`
	PlayerAlias string `json:"player_alias"`
}

// ErrUnknownItem 表示解析后某个物品名不在 alias 表里（严格模式）。
type ErrUnknownItem struct {
	Names []string
}

func (e *ErrUnknownItem) Error() string {
	return fmt.Sprintf("unknown item alias(es): %v", e.Names)
}

// ErrNewAliasRequired 表示前端没传 is_new_alias 但库里没该 player_alias 的仓库。
var ErrNewAliasRequired = errors.New("new player_alias requires confirmation")

// Ingest 主入口。
func Ingest(ctx context.Context, st *store.Store, rawText, uploaderID, playerAlias string, isNewAlias bool) (*Result, error) {
	if uploaderID == "" {
		return nil, errors.New("uploader required")
	}
	if playerAlias == "" {
		return nil, errors.New("player_alias required")
	}
	parsed, err := parser.Parse(rawText)
	if err != nil {
		return nil, err
	}

	// 校验所有物品都在 alias 表里。
	cidByLine := make([]string, len(parsed.Items))
	var unknown []string
	for i, it := range parsed.Items {
		cid, ok, err := st.ResolveAlias(it.Name)
		if err != nil {
			return nil, err
		}
		if !ok {
			unknown = append(unknown, it.Name)
			continue
		}
		cidByLine[i] = cid
	}
	if len(unknown) > 0 {
		return nil, &ErrUnknownItem{Names: unknown}
	}

	warID, err := st.EnsureActiveWar()
	if err != nil {
		return nil, err
	}

	// 仓库存在性 / 新别名确认。
	existing, err := st.FindStockpileByKey(warID, parsed.Header.Region, parsed.Header.SubRegion,
		parsed.Header.Building, parsed.Header.Visibility, playerAlias)
	if err != nil && !isNoRows(err) {
		return nil, err
	}
	if existing == nil && !isNewAlias {
		return nil, ErrNewAliasRequired
	}

	stockpileID, err := st.UpsertStockpile(&store.Stockpile{
		WarID:        warID,
		Region:       parsed.Header.Region,
		SubRegion:    parsed.Header.SubRegion,
		BuildingName: parsed.Header.Building,
		Visibility:   parsed.Header.Visibility,
		PlayerAlias:  playerAlias,
	})
	if err != nil {
		return nil, err
	}

	snapItems := make([]store.SnapshotItem, len(parsed.Items))
	for i, it := range parsed.Items {
		snapItems[i] = store.SnapshotItem{
			CanonicalID: cidByLine[i],
			IsCrate:     it.IsCrate,
			Quantity:    it.Quantity,
		}
	}

	snap := store.Snapshot{
		StockpileID: stockpileID,
		WarID:       warID,
		UploaderID:  uploaderID,
		CapturedAt:  parsed.Header.CapturedAt.Unix(),
		UploadedAt:  time.Now().Unix(),
	}
	snap.PosX.Float64, snap.PosX.Valid = parsed.Header.PosX, true
	snap.PosY.Float64, snap.PosY.Valid = parsed.Header.PosY, true

	snapID, dup, err := st.CreateSnapshot(ctx, snap, rawText, snapItems)
	if err != nil {
		return nil, err
	}

	return &Result{
		StockpileID: stockpileID,
		SnapshotID:  snapID,
		WarID:       warID,
		ItemCount:   len(parsed.Items),
		Duplicate:   dup,
		CapturedAt:  parsed.Header.CapturedAt.Unix(),
		Region:      parsed.Header.Region,
		SubRegion:   parsed.Header.SubRegion,
		Building:    parsed.Header.Building,
		Visibility:  parsed.Header.Visibility,
		PlayerAlias: playerAlias,
	}, nil
}

func isNoRows(err error) bool {
	return err != nil && err.Error() == "sql: no rows in result set"
}