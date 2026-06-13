// Package store 包装 SQLite 访问。
//
// 设计：纯 database/sql + 手写 SQL，不引 ORM；
// 单例 *Store 持有 *sql.DB，迁移与种子内嵌为字面量字符串。
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

//go:embed seeds/*.sql
var seedsFS embed.FS

type Store struct {
	DB *sql.DB
}

// Open 打开（或创建）SQLite 数据库，启用 WAL。
func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_time_format=sqlite"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

// Migrate 顺序执行 migrations/*.sql。
func (s *Store) Migrate() error {
	return runScripts(s.DB, migrationsFS, "migrations")
}

// Seed 顺序执行 seeds/*.sql；items 表非空则跳过。
func (s *Store) Seed() error {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM items`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return runScripts(s.DB, seedsFS, "seeds")
}

func runScripts(db *sql.DB, dir fs.FS, root string) error {
	ents, err := fs.ReadDir(dir, root)
	if err != nil {
		return err
	}
	files := make([]string, 0, len(ents))
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	for _, name := range files {
		raw, err := fs.ReadFile(dir, root+"/"+name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := db.Exec(string(raw)); err != nil {
			return fmt.Errorf("exec %s: %w", name, err)
		}
	}
	return nil
}

// EnsureOwner 确保 owner Steam64ID 在 users 表里存在并标记为 owner。
// 第一次启动会插入一条 must_reset_pw=1 的占位记录。
func (s *Store) EnsureOwner(steamID string) error {
	if steamID == "" {
		return errors.New("config.owner_steam64id is empty")
	}
	now := time.Now().Unix()
	_, err := s.DB.Exec(
		`INSERT INTO users (steam64id, display_name, is_owner, is_active, must_reset_pw, created_at)
		 VALUES (?, ?, 1, 1, 1, ?)
		 ON CONFLICT(steam64id) DO UPDATE SET is_owner=1, is_active=1`,
		steamID, "Owner-"+steamID[len(steamID)-4:], now,
	)
	return err
}

// EnsureActiveWar 没有 active war 时，自动创建一场 'Default War'。
func (s *Store) EnsureActiveWar() (warID int64, err error) {
	row := s.DB.QueryRow(`SELECT id FROM wars WHERE is_active=1 LIMIT 1`)
	if err = row.Scan(&warID); err == nil {
		return warID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	now := time.Now().Unix()
	res, err := s.DB.Exec(
		`INSERT INTO wars (label, started_at, is_active) VALUES (?, ?, 1)`,
		"Default War", now,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ===== Users =====

type User struct {
	Steam64ID    string
	DisplayName  string
	PasswordHash sql.NullString
	IsOwner      bool
	IsActive     bool
	MustResetPW  bool
	CreatedAt    int64
	RegisteredAt sql.NullInt64
}

func (s *Store) GetUser(steamID string) (*User, error) {
	row := s.DB.QueryRow(
		`SELECT steam64id, display_name, password_hash, is_owner, is_active, must_reset_pw, created_at, registered_at
		 FROM users WHERE steam64id=?`, steamID)
	u := &User{}
	if err := row.Scan(&u.Steam64ID, &u.DisplayName, &u.PasswordHash,
		&u.IsOwner, &u.IsActive, &u.MustResetPW, &u.CreatedAt, &u.RegisteredAt); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) ListUsers() ([]*User, error) {
	rows, err := s.DB.Query(
		`SELECT steam64id, display_name, password_hash, is_owner, is_active, must_reset_pw, created_at, registered_at
		 FROM users ORDER BY is_owner DESC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*User{}
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.Steam64ID, &u.DisplayName, &u.PasswordHash,
			&u.IsOwner, &u.IsActive, &u.MustResetPW, &u.CreatedAt, &u.RegisteredAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// AddWhitelist 把一个 Steam64ID 加入白名单（已存在则只更新 display_name+is_active）。
func (s *Store) AddWhitelist(steamID, displayName string) error {
	if displayName == "" {
		displayName = "Member-" + steamID[len(steamID)-4:]
	}
	_, err := s.DB.Exec(
		`INSERT INTO users (steam64id, display_name, is_owner, is_active, must_reset_pw, created_at)
		 VALUES (?, ?, 0, 1, 0, ?)
		 ON CONFLICT(steam64id) DO UPDATE SET is_active=1, display_name=COALESCE(NULLIF(?,''), users.display_name)`,
		steamID, displayName, time.Now().Unix(), displayName,
	)
	return err
}

// RegisterPassword 完成注册：仅当 password_hash 为 NULL 或 must_reset_pw=1 时允许。
func (s *Store) RegisterPassword(steamID, hash, displayName string) error {
	now := time.Now().Unix()
	res, err := s.DB.Exec(
		`UPDATE users
		 SET password_hash=?, display_name=COALESCE(NULLIF(?,''), display_name),
		     must_reset_pw=0, registered_at=COALESCE(registered_at, ?)
		 WHERE steam64id=? AND is_active=1 AND (password_hash IS NULL OR must_reset_pw=1)`,
		hash, displayName, now, steamID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("not in whitelist or already registered")
	}
	return nil
}

func (s *Store) UpdatePassword(steamID, hash string) error {
	_, err := s.DB.Exec(`UPDATE users SET password_hash=?, must_reset_pw=0 WHERE steam64id=?`, hash, steamID)
	return err
}

func (s *Store) MarkResetPassword(steamID string) error {
	_, err := s.DB.Exec(`UPDATE users SET must_reset_pw=1 WHERE steam64id=?`, steamID)
	return err
}

func (s *Store) KickUser(steamID string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE users SET is_active=0 WHERE steam64id=?`, steamID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE steam64id=?`, steamID); err != nil {
		return err
	}
	return tx.Commit()
}

// ===== Sessions =====

type Session struct {
	Token      string
	Steam64ID  string
	CreatedAt  int64
	ExpiresAt  int64
	LastSeen   int64
	UserAgent  string
}

func (s *Store) CreateSession(token, steamID, ua string, ttl time.Duration) error {
	now := time.Now().Unix()
	_, err := s.DB.Exec(
		`INSERT INTO sessions (token, steam64id, created_at, expires_at, last_seen, user_agent) VALUES (?,?,?,?,?,?)`,
		token, steamID, now, now+int64(ttl.Seconds()), now, ua,
	)
	return err
}

func (s *Store) ResolveSession(token string) (*Session, *User, error) {
	row := s.DB.QueryRow(
		`SELECT s.token, s.steam64id, s.created_at, s.expires_at, s.last_seen, s.user_agent,
		        u.steam64id, u.display_name, u.password_hash, u.is_owner, u.is_active, u.must_reset_pw, u.created_at, u.registered_at
		 FROM sessions s JOIN users u ON u.steam64id = s.steam64id
		 WHERE s.token=?`, token)
	sess := &Session{}
	user := &User{}
	if err := row.Scan(&sess.Token, &sess.Steam64ID, &sess.CreatedAt, &sess.ExpiresAt, &sess.LastSeen, &sess.UserAgent,
		&user.Steam64ID, &user.DisplayName, &user.PasswordHash, &user.IsOwner, &user.IsActive, &user.MustResetPW, &user.CreatedAt, &user.RegisteredAt); err != nil {
		return nil, nil, err
	}
	return sess, user, nil
}

func (s *Store) TouchSession(token string) {
	_, _ = s.DB.Exec(`UPDATE sessions SET last_seen=? WHERE token=?`, time.Now().Unix(), token)
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.DB.Exec(`DELETE FROM sessions WHERE token=?`, token)
	return err
}

func (s *Store) GarbageCollectSessions() {
	_, _ = s.DB.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix())
}

// ===== Wars =====

type War struct {
	ID        int64
	Label     string
	StartedAt int64
	EndedAt   sql.NullInt64
	IsActive  bool
	Notes     sql.NullString
}

func (s *Store) ListWars() ([]*War, error) {
	rows, err := s.DB.Query(`SELECT id, label, started_at, ended_at, is_active, notes FROM wars ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*War{}
	for rows.Next() {
		w := &War{}
		if err := rows.Scan(&w.ID, &w.Label, &w.StartedAt, &w.EndedAt, &w.IsActive, &w.Notes); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) GetWar(id int64) (*War, error) {
	row := s.DB.QueryRow(`SELECT id, label, started_at, ended_at, is_active, notes FROM wars WHERE id=?`, id)
	w := &War{}
	err := row.Scan(&w.ID, &w.Label, &w.StartedAt, &w.EndedAt, &w.IsActive, &w.Notes)
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (s *Store) GetActiveWar() (*War, error) {
	row := s.DB.QueryRow(`SELECT id, label, started_at, ended_at, is_active, notes FROM wars WHERE is_active=1 LIMIT 1`)
	w := &War{}
	err := row.Scan(&w.ID, &w.Label, &w.StartedAt, &w.EndedAt, &w.IsActive, &w.Notes)
	if err != nil {
		return nil, err
	}
	return w, nil
}

// StartNewWar 自动归档旧 active；新 war is_active=1。
func (s *Store) StartNewWar(label, notes string) (int64, error) {
	now := time.Now().Unix()
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE wars SET is_active=0, ended_at=COALESCE(ended_at,?) WHERE is_active=1`, now); err != nil {
		return 0, err
	}
	res, err := tx.Exec(`INSERT INTO wars (label, started_at, is_active, notes) VALUES (?,?,1,?)`, label, now, notes)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) EndWar(id int64) error {
	_, err := s.DB.Exec(`UPDATE wars SET is_active=0, ended_at=COALESCE(ended_at,?) WHERE id=?`, time.Now().Unix(), id)
	return err
}

func (s *Store) DeleteWar(id int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM snapshot_items WHERE snapshot_id IN (SELECT id FROM snapshots WHERE war_id=?)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM snapshots WHERE war_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM stockpiles WHERE war_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM wars WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ===== Items / Aliases =====

type Item struct {
	CanonicalID string
	NameZH      string
	NameEN      sql.NullString
	Faction     sql.NullString
	Category    sql.NullString
	CrateSize   sql.NullInt64
	IconPath    sql.NullString
	IconSource  string
	WikiURL     sql.NullString
	UpdatedAt   int64
}

func (s *Store) ResolveAlias(alias string) (string, bool, error) {
	var cid string
	err := s.DB.QueryRow(`SELECT canonical_id FROM item_aliases WHERE alias=?`, alias).Scan(&cid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return cid, true, nil
}

func (s *Store) ListItems() ([]*Item, error) {
	rows, err := s.DB.Query(`SELECT canonical_id, name_zh, name_en, faction, category, crate_size, icon_path, icon_source, wiki_url, updated_at FROM items ORDER BY category, name_zh`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Item{}
	for rows.Next() {
		it := &Item{}
		if err := rows.Scan(&it.CanonicalID, &it.NameZH, &it.NameEN, &it.Faction, &it.Category, &it.CrateSize, &it.IconPath, &it.IconSource, &it.WikiURL, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) GetItem(cid string) (*Item, error) {
	row := s.DB.QueryRow(`SELECT canonical_id, name_zh, name_en, faction, category, crate_size, icon_path, icon_source, wiki_url, updated_at FROM items WHERE canonical_id=?`, cid)
	it := &Item{}
	if err := row.Scan(&it.CanonicalID, &it.NameZH, &it.NameEN, &it.Faction, &it.Category, &it.CrateSize, &it.IconPath, &it.IconSource, &it.WikiURL, &it.UpdatedAt); err != nil {
		return nil, err
	}
	return it, nil
}

// UpsertItem 创建或更新一个 item，同时确保 alias=name_zh 存在。
func (s *Store) UpsertItem(it *Item, aliases []string) error {
	now := time.Now().Unix()
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO items (canonical_id, name_zh, name_en, faction, category, crate_size, icon_path, icon_source, wiki_url, updated_at)
		 VALUES (?,?,?,?,?,?,?,COALESCE(?,'placeholder'),?,?)
		 ON CONFLICT(canonical_id) DO UPDATE SET
		   name_zh=excluded.name_zh, name_en=excluded.name_en, faction=excluded.faction,
		   category=excluded.category, crate_size=excluded.crate_size,
		   icon_path=COALESCE(excluded.icon_path, items.icon_path),
		   icon_source=excluded.icon_source,
		   wiki_url=excluded.wiki_url,
		   updated_at=excluded.updated_at`,
		it.CanonicalID, it.NameZH, it.NameEN, it.Faction, it.Category, it.CrateSize, it.IconPath, it.IconSource, it.WikiURL, now,
	); err != nil {
		return err
	}
	allAliases := append([]string{it.NameZH}, aliases...)
	for _, a := range allAliases {
		if a == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO item_aliases (alias, canonical_id) VALUES (?, ?)`, a, it.CanonicalID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpdateItemIcon(cid, iconPath, source string) error {
	_, err := s.DB.Exec(`UPDATE items SET icon_path=?, icon_source=?, updated_at=? WHERE canonical_id=?`,
		iconPath, source, time.Now().Unix(), cid)
	return err
}

// ===== Stockpiles & Snapshots =====

type Stockpile struct {
	ID            int64
	WarID         int64
	Region        string
	SubRegion     string
	BuildingName  string
	Visibility    string
	PlayerAlias   string
	FirstSeenAt   int64
	LastSeenAt    int64
}

type StockpileWithMeta struct {
	Stockpile
	UploaderID    string
	UploaderName  string
	LastCapturedAt int64
}

// FindStockpileByKey 唯一键查找。
func (s *Store) FindStockpileByKey(warID int64, region, subRegion, building, visibility, alias string) (*Stockpile, error) {
	row := s.DB.QueryRow(
		`SELECT id, war_id, region, sub_region, building_name, visibility, player_alias, first_seen_at, last_seen_at
		 FROM stockpiles WHERE war_id=? AND region=? AND sub_region=? AND building_name=? AND visibility=? AND player_alias=?`,
		warID, region, subRegion, building, visibility, alias)
	sp := &Stockpile{}
	if err := row.Scan(&sp.ID, &sp.WarID, &sp.Region, &sp.SubRegion, &sp.BuildingName, &sp.Visibility, &sp.PlayerAlias, &sp.FirstSeenAt, &sp.LastSeenAt); err != nil {
		return nil, err
	}
	return sp, nil
}

func (s *Store) UpsertStockpile(sp *Stockpile) (int64, error) {
	now := time.Now().Unix()
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO stockpiles (war_id, region, sub_region, building_name, visibility, player_alias, first_seen_at, last_seen_at)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON CONFLICT(war_id, region, sub_region, building_name, visibility, player_alias)
		 DO UPDATE SET last_seen_at=excluded.last_seen_at`,
		sp.WarID, sp.Region, sp.SubRegion, sp.BuildingName, sp.Visibility, sp.PlayerAlias, now, now,
	); err != nil {
		return 0, err
	}
	// 不依赖 LastInsertId（modernc.org/sqlite 的 DO UPDATE 不会清零 last_insert_rowid，
	// 可能返回前一条无关 INSERT 的 rowid）。统一通过唯一键反查。
	var id int64
	if err := tx.QueryRow(
		`SELECT id FROM stockpiles WHERE war_id=? AND region=? AND sub_region=? AND building_name=? AND visibility=? AND player_alias=?`,
		sp.WarID, sp.Region, sp.SubRegion, sp.BuildingName, sp.Visibility, sp.PlayerAlias,
	).Scan(&id); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

type SnapshotItem struct {
	CanonicalID string
	IsCrate     bool
	Quantity    int64
}

type Snapshot struct {
	ID           int64
	StockpileID  int64
	WarID        int64
	UploaderID   string
	CapturedAt   int64
	UploadedAt   int64
	PosX         sql.NullFloat64
	PosY         sql.NullFloat64
	RawSHA256    string
}

// CreateSnapshot 写一条 snapshot + 批量 snapshot_items。
// 命中 (stockpile_id, raw_sha256) dedup 时，返回已存在的 snapshot id 和 dup=true。
func (s *Store) CreateSnapshot(ctx context.Context, snap Snapshot, rawText string, items []SnapshotItem) (snapID int64, dup bool, err error) {
	sum := sha256.Sum256([]byte(rawText))
	snap.RawSHA256 = hex.EncodeToString(sum[:])

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO snapshots (stockpile_id, war_id, uploader_id, captured_at, uploaded_at, pos_x, pos_y, raw_sha256, raw_text)
		 VALUES (?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(stockpile_id, raw_sha256) DO NOTHING`,
		snap.StockpileID, snap.WarID, snap.UploaderID, snap.CapturedAt, snap.UploadedAt, snap.PosX, snap.PosY, snap.RawSHA256, rawText,
	)
	if err != nil {
		return 0, false, err
	}
	rowsAff, _ := res.RowsAffected()
	if rowsAff == 0 {
		var existing int64
		if err := tx.QueryRowContext(ctx,
			`SELECT id FROM snapshots WHERE stockpile_id=? AND raw_sha256=?`,
			snap.StockpileID, snap.RawSHA256).Scan(&existing); err != nil {
			return 0, false, err
		}
		if err := tx.Commit(); err != nil {
			return 0, false, err
		}
		return existing, true, nil
	}
	snapID, err = res.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO snapshot_items (snapshot_id, canonical_id, is_crate, quantity) VALUES (?,?,?,?)`)
	if err != nil {
		return 0, false, err
	}
	defer stmt.Close()
	for _, it := range items {
		crate := 0
		if it.IsCrate {
			crate = 1
		}
		if _, err := stmt.ExecContext(ctx, snapID, it.CanonicalID, crate, it.Quantity); err != nil {
			return 0, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return snapID, false, nil
}

type StockpileSummary struct {
	Stockpile
	LastUploaderID   string
	LastUploaderName string
	LastCapturedAt   int64
	LastUploadedAt   int64
	SnapshotID       int64
	ItemCount        int64
}

// ListStockpilesByWar 列出某 war 下所有仓 + 最新 snapshot 元信息。
func (s *Store) ListStockpilesByWar(warID int64) ([]*StockpileSummary, error) {
	rows, err := s.DB.Query(`
		SELECT sp.id, sp.war_id, sp.region, sp.sub_region, sp.building_name, sp.visibility, sp.player_alias,
		       sp.first_seen_at, sp.last_seen_at,
		       COALESCE(latest.uploader_id, '') AS uploader_id,
		       COALESCE(u.display_name, '') AS uploader_name,
		       COALESCE(latest.captured_at, 0) AS captured_at,
		       COALESCE(latest.uploaded_at, 0) AS uploaded_at,
		       COALESCE(latest.id, 0) AS snap_id,
		       COALESCE(ic.cnt, 0) AS item_count
		FROM stockpiles sp
		LEFT JOIN (
		  SELECT s.* FROM snapshots s
		  JOIN (SELECT stockpile_id, MAX(captured_at) AS m FROM snapshots GROUP BY stockpile_id) t
		    ON t.stockpile_id = s.stockpile_id AND t.m = s.captured_at
		) latest ON latest.stockpile_id = sp.id
		LEFT JOIN users u ON u.steam64id = latest.uploader_id
		LEFT JOIN (SELECT snapshot_id, COUNT(1) AS cnt FROM snapshot_items GROUP BY snapshot_id) ic ON ic.snapshot_id = latest.id
		WHERE sp.war_id=?
		ORDER BY sp.last_seen_at DESC`, warID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*StockpileSummary{}
	for rows.Next() {
		ss := &StockpileSummary{}
		if err := rows.Scan(&ss.ID, &ss.WarID, &ss.Region, &ss.SubRegion, &ss.BuildingName, &ss.Visibility, &ss.PlayerAlias,
			&ss.FirstSeenAt, &ss.LastSeenAt,
			&ss.LastUploaderID, &ss.LastUploaderName, &ss.LastCapturedAt, &ss.LastUploadedAt,
			&ss.SnapshotID, &ss.ItemCount); err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

func (s *Store) GetStockpile(id int64) (*Stockpile, error) {
	row := s.DB.QueryRow(
		`SELECT id, war_id, region, sub_region, building_name, visibility, player_alias, first_seen_at, last_seen_at
		 FROM stockpiles WHERE id=?`, id)
	sp := &Stockpile{}
	if err := row.Scan(&sp.ID, &sp.WarID, &sp.Region, &sp.SubRegion, &sp.BuildingName, &sp.Visibility, &sp.PlayerAlias, &sp.FirstSeenAt, &sp.LastSeenAt); err != nil {
		return nil, err
	}
	return sp, nil
}

type LatestItemRow struct {
	CanonicalID string
	NameZH      string
	NameEN      sql.NullString
	Category    sql.NullString
	CrateSize   sql.NullInt64
	IconSource  string
	Loose       int64 // is_crate=0
	Crates      int64 // is_crate=1
}

// GetLatestItemsByStockpile 单仓最新 snapshot 的物品聚合（散件/箱装两列展开）。
func (s *Store) GetLatestItemsByStockpile(stockpileID int64) ([]*LatestItemRow, int64, error) {
	var snapID int64
	var capturedAt int64
	err := s.DB.QueryRow(
		`SELECT id, captured_at FROM snapshots WHERE stockpile_id=? ORDER BY captured_at DESC LIMIT 1`, stockpileID).
		Scan(&snapID, &capturedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return []*LatestItemRow{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.Query(`
		SELECT i.canonical_id, i.name_zh, i.name_en, i.category, i.crate_size, i.icon_source,
		       SUM(CASE WHEN si.is_crate=0 THEN si.quantity ELSE 0 END) AS loose,
		       SUM(CASE WHEN si.is_crate=1 THEN si.quantity ELSE 0 END) AS crates
		FROM snapshot_items si
		JOIN items i ON i.canonical_id = si.canonical_id
		WHERE si.snapshot_id = ?
		GROUP BY i.canonical_id
		ORDER BY i.category, i.name_zh`, snapID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*LatestItemRow{}
	for rows.Next() {
		r := &LatestItemRow{}
		if err := rows.Scan(&r.CanonicalID, &r.NameZH, &r.NameEN, &r.Category, &r.CrateSize, &r.IconSource, &r.Loose, &r.Crates); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, capturedAt, rows.Err()
}

// TimelinePoint (timestamp_unix, qty)
type TimelinePoint struct {
	T int64
	Q int64
}

// GetTimeline 单仓单物品时间序列（散件 / 箱装分两条）。
func (s *Store) GetTimeline(stockpileID int64, canonicalID string) (loose []TimelinePoint, crates []TimelinePoint, err error) {
	rows, err := s.DB.Query(`
		SELECT s.captured_at, si.is_crate, si.quantity
		FROM snapshot_items si
		JOIN snapshots s ON s.id = si.snapshot_id
		WHERE s.stockpile_id=? AND si.canonical_id=?
		ORDER BY s.captured_at ASC`, stockpileID, canonicalID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t, qty int64
		var crate int
		if err := rows.Scan(&t, &crate, &qty); err != nil {
			return nil, nil, err
		}
		if crate == 1 {
			crates = append(crates, TimelinePoint{T: t, Q: qty})
		} else {
			loose = append(loose, TimelinePoint{T: t, Q: qty})
		}
	}
	return loose, crates, rows.Err()
}

type ItemAggregate struct {
	StockpileID    int64
	Region         string
	BuildingName   string
	PlayerAlias    string
	Loose          int64
	Crates         int64
	LastCapturedAt int64
}

// GetItemAcrossStockpiles 单 item 在某 war 下跨所有仓的当前态。
func (s *Store) GetItemAcrossStockpiles(warID int64, canonicalID string) ([]*ItemAggregate, error) {
	rows, err := s.DB.Query(`
		WITH latest AS (
			SELECT s.id, s.stockpile_id, s.captured_at
			FROM snapshots s
			JOIN (SELECT stockpile_id, MAX(captured_at) m FROM snapshots WHERE war_id=? GROUP BY stockpile_id) t
			  ON t.stockpile_id = s.stockpile_id AND t.m = s.captured_at
		)
		SELECT sp.id, sp.region, sp.building_name, sp.player_alias,
		       SUM(CASE WHEN si.is_crate=0 THEN si.quantity ELSE 0 END) AS loose,
		       SUM(CASE WHEN si.is_crate=1 THEN si.quantity ELSE 0 END) AS crates,
		       latest.captured_at
		FROM latest
		JOIN stockpiles sp ON sp.id = latest.stockpile_id
		LEFT JOIN snapshot_items si ON si.snapshot_id = latest.id AND si.canonical_id=?
		WHERE sp.war_id=?
		GROUP BY sp.id, latest.captured_at
		ORDER BY (loose + crates) DESC`, warID, canonicalID, warID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ItemAggregate{}
	for rows.Next() {
		ag := &ItemAggregate{}
		if err := rows.Scan(&ag.StockpileID, &ag.Region, &ag.BuildingName, &ag.PlayerAlias, &ag.Loose, &ag.Crates, &ag.LastCapturedAt); err != nil {
			return nil, err
		}
		out = append(out, ag)
	}
	return out, rows.Err()
}

type RecentSnapshot struct {
	ID             int64
	StockpileID    int64
	Region         string
	BuildingName   string
	PlayerAlias    string
	UploaderID     string
	UploaderName   string
	CapturedAt     int64
	UploadedAt     int64
	ItemCount      int64
}

func (s *Store) RecentSnapshots(limit int) ([]*RecentSnapshot, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.DB.Query(`
		SELECT s.id, s.stockpile_id, sp.region, sp.building_name, sp.player_alias,
		       s.uploader_id, COALESCE(u.display_name,''), s.captured_at, s.uploaded_at,
		       (SELECT COUNT(1) FROM snapshot_items si WHERE si.snapshot_id = s.id)
		FROM snapshots s
		JOIN stockpiles sp ON sp.id = s.stockpile_id
		LEFT JOIN users u ON u.steam64id = s.uploader_id
		ORDER BY s.uploaded_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*RecentSnapshot{}
	for rows.Next() {
		r := &RecentSnapshot{}
		if err := rows.Scan(&r.ID, &r.StockpileID, &r.Region, &r.BuildingName, &r.PlayerAlias,
			&r.UploaderID, &r.UploaderName, &r.CapturedAt, &r.UploadedAt, &r.ItemCount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type AdminStats struct {
	UserCount         int64
	ActiveWarLabel    string
	ActiveWarStartedAt int64
	ItemCount         int64
	PlaceholderItems  int64
	StockpileCount    int64
	SnapshotCount     int64
	DBPath            string
}

func (s *Store) GetStats() (*AdminStats, error) {
	st := &AdminStats{}
	_ = s.DB.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&st.UserCount)
	_ = s.DB.QueryRow(`SELECT COUNT(1) FROM items`).Scan(&st.ItemCount)
	_ = s.DB.QueryRow(`SELECT COUNT(1) FROM items WHERE icon_source='placeholder'`).Scan(&st.PlaceholderItems)
	_ = s.DB.QueryRow(`SELECT COUNT(1) FROM stockpiles`).Scan(&st.StockpileCount)
	_ = s.DB.QueryRow(`SELECT COUNT(1) FROM snapshots`).Scan(&st.SnapshotCount)
	var label sql.NullString
	var started sql.NullInt64
	_ = s.DB.QueryRow(`SELECT label, started_at FROM wars WHERE is_active=1`).Scan(&label, &started)
	st.ActiveWarLabel = label.String
	st.ActiveWarStartedAt = started.Int64
	return st, nil
}