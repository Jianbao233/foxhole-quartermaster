-- Foxhole Quartermaster: 初始 schema
-- 与 DESIGN.md §3.1 对齐

PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

CREATE TABLE IF NOT EXISTS users (
  steam64id      TEXT PRIMARY KEY,
  display_name   TEXT NOT NULL,
  password_hash  TEXT,
  is_owner       INTEGER NOT NULL DEFAULT 0,
  is_active      INTEGER NOT NULL DEFAULT 1,
  must_reset_pw  INTEGER NOT NULL DEFAULT 0,
  created_at     INTEGER NOT NULL,
  registered_at  INTEGER
);

CREATE TABLE IF NOT EXISTS sessions (
  token       TEXT PRIMARY KEY,
  steam64id   TEXT NOT NULL REFERENCES users(steam64id),
  created_at  INTEGER NOT NULL,
  expires_at  INTEGER NOT NULL,
  last_seen   INTEGER NOT NULL,
  user_agent  TEXT
);
CREATE INDEX IF NOT EXISTS idx_sessions_uid ON sessions(steam64id);

CREATE TABLE IF NOT EXISTS wars (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  label       TEXT NOT NULL,
  started_at  INTEGER NOT NULL,
  ended_at    INTEGER,
  is_active   INTEGER NOT NULL DEFAULT 0,
  notes       TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_wars_active_unique
  ON wars(is_active) WHERE is_active = 1;

CREATE TABLE IF NOT EXISTS items (
  canonical_id TEXT PRIMARY KEY,
  name_zh      TEXT NOT NULL,
  name_en      TEXT,
  faction      TEXT,
  category     TEXT,
  crate_size   INTEGER,
  icon_path    TEXT,
  icon_source  TEXT NOT NULL DEFAULT 'placeholder',
  wiki_url     TEXT,
  updated_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS item_aliases (
  alias        TEXT PRIMARY KEY,
  canonical_id TEXT NOT NULL REFERENCES items(canonical_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_item_aliases_cid ON item_aliases(canonical_id);

CREATE TABLE IF NOT EXISTS stockpiles (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  war_id        INTEGER NOT NULL REFERENCES wars(id),
  region        TEXT NOT NULL,
  sub_region    TEXT NOT NULL,
  building_name TEXT NOT NULL,
  visibility    TEXT NOT NULL,
  player_alias  TEXT NOT NULL,
  first_seen_at INTEGER NOT NULL,
  last_seen_at  INTEGER NOT NULL,
  UNIQUE (war_id, region, sub_region, building_name, visibility, player_alias)
);
CREATE INDEX IF NOT EXISTS idx_stockpiles_war ON stockpiles(war_id);

CREATE TABLE IF NOT EXISTS snapshots (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  stockpile_id INTEGER NOT NULL REFERENCES stockpiles(id) ON DELETE CASCADE,
  war_id       INTEGER NOT NULL,
  uploader_id  TEXT NOT NULL REFERENCES users(steam64id),
  captured_at  INTEGER NOT NULL,
  uploaded_at  INTEGER NOT NULL,
  pos_x        REAL,
  pos_y        REAL,
  raw_sha256   TEXT NOT NULL,
  raw_text     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_snap_stockpile_time ON snapshots(stockpile_id, captured_at DESC);
CREATE INDEX IF NOT EXISTS idx_snap_war ON snapshots(war_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_snap_dedup ON snapshots(stockpile_id, raw_sha256);

CREATE TABLE IF NOT EXISTS snapshot_items (
  snapshot_id  INTEGER NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  canonical_id TEXT NOT NULL REFERENCES items(canonical_id),
  is_crate     INTEGER NOT NULL,
  quantity     INTEGER NOT NULL,
  PRIMARY KEY (snapshot_id, canonical_id, is_crate)
);
CREATE INDEX IF NOT EXISTS idx_snap_items_item ON snapshot_items(canonical_id);

CREATE VIEW IF NOT EXISTS v_latest_snapshot AS
SELECT s.*
FROM snapshots s
JOIN (
  SELECT stockpile_id, MAX(captured_at) AS m
  FROM snapshots
  GROUP BY stockpile_id
) t ON t.stockpile_id = s.stockpile_id AND t.m = s.captured_at;