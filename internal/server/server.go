// Package server 组装 chi router、JSON handler、icon serve 与前端 fallback。
package server

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Jianbao233/foxhole-quartermaster/internal/auth"
	"github.com/Jianbao233/foxhole-quartermaster/internal/config"
	"github.com/Jianbao233/foxhole-quartermaster/internal/ingest"
	"github.com/Jianbao233/foxhole-quartermaster/internal/parser"
	"github.com/Jianbao233/foxhole-quartermaster/internal/store"
)

//go:embed devstub/index.html
var devStubFS embed.FS

// NoEmbedFS 在未编译 web/dist 时返回 dev 占位页文件系统。
func NoEmbedFS() fs.FS {
	sub, _ := fs.Sub(devStubFS, "devstub")
	return sub
}

type Server struct {
	cfg   *config.Config
	store *store.Store
	web   fs.FS
}

func New(cfg *config.Config, st *store.Store, webFS fs.FS) http.Handler {
	s := &Server{cfg: cfg, store: st, web: webFS}

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(auth.Middleware(st))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/register", s.handleRegister)
		r.Post("/auth/logout", s.handleLogout)
		r.Get("/auth/me", s.handleMe)

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireMember)
			r.Post("/ingest", s.handleIngest)
			r.Get("/wars", s.handleListWars)
			r.Get("/wars/active", s.handleActiveWar)
			r.Get("/stockpiles", s.handleListStockpiles)
			r.Get("/stockpiles/{id}", s.handleStockpileDetail)
			r.Get("/stockpiles/{id}/timeline", s.handleStockpileTimeline)
			r.Get("/items", s.handleListItems)
			r.Get("/items/{cid}", s.handleItemDetail)
			r.Get("/uploads", s.handleRecentUploads)
		})

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireMember)
			r.Use(auth.RequireOwner)
			r.Get("/admin/stats", s.handleAdminStats)
			r.Get("/admin/users", s.handleAdminListUsers)
			r.Post("/admin/whitelist", s.handleAdminAddWhitelist)
			r.Post("/admin/users/{id}/reset", s.handleAdminResetUser)
			r.Post("/admin/users/{id}/kick", s.handleAdminKickUser)
			r.Post("/admin/wars/new", s.handleAdminNewWar)
			r.Post("/admin/wars/{id}/end", s.handleAdminEndWar)
			r.Post("/admin/wars/{id}/delete", s.handleAdminDeleteWar)
			r.Get("/admin/items", s.handleAdminListItems)
			r.Post("/admin/items", s.handleAdminUpsertItem)
			r.Post("/admin/items/{cid}/icon", s.handleAdminUploadIcon)
			r.Get("/admin/war/{id}/export", s.handleAdminExportWar)
		})
	})

	r.Get("/icons/{name}", s.handleIcon)

	// 前端：未匹配的路由全部交给 SPA index.html。
	r.NotFound(s.handleSPA)
	r.Get("/*", s.handleSPA)

	return r
}

// ===== helpers =====

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": msg},
	})
}

func parseInt64(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }

// ===== auth =====

type loginReq struct {
	Steam64ID string `json:"steam64id"`
	Password  string `json:"password"`
}

func validSteam64(id string) bool {
	if len(id) != 17 {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "bad_request", "invalid json")
		return
	}
	if !validSteam64(req.Steam64ID) || req.Password == "" {
		writeErr(w, 400, "bad_request", "steam64id or password missing")
		return
	}
	user, err := s.store.GetUser(req.Steam64ID)
	if err != nil || user == nil || !user.IsActive {
		writeErr(w, 401, "invalid_credentials", "用户名或密码错误")
		return
	}
	if user.MustResetPW {
		writeErr(w, 403, "must_reset", "owner 已要求你重置密码，请到注册页重新设密")
		return
	}
	if !user.PasswordHash.Valid || !auth.Verify(user.PasswordHash.String, req.Password) {
		writeErr(w, 401, "invalid_credentials", "用户名或密码错误")
		return
	}
	s.issueSession(w, r, user.Steam64ID)
	writeJSON(w, 200, map[string]any{"user": userPublic(user)})
}

type registerReq struct {
	Steam64ID   string `json:"steam64id"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "bad_request", "invalid json")
		return
	}
	if !validSteam64(req.Steam64ID) || len(req.Password) < 6 {
		writeErr(w, 400, "bad_request", "steam64id 必须 17 位数字；密码至少 6 位")
		return
	}
	user, err := s.store.GetUser(req.Steam64ID)
	if err != nil {
		writeErr(w, 403, "not_in_whitelist", "该 Steam64ID 不在白名单内")
		return
	}
	if !user.IsActive {
		writeErr(w, 403, "kicked", "账号已被踢出")
		return
	}
	if user.PasswordHash.Valid && !user.MustResetPW {
		writeErr(w, 409, "already_registered", "该账号已注册，请直接登录")
		return
	}
	hash, err := auth.Hash(req.Password)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	if err := s.store.RegisterPassword(req.Steam64ID, hash, strings.TrimSpace(req.DisplayName)); err != nil {
		writeErr(w, 403, "register_failed", err.Error())
		return
	}
	user, _ = s.store.GetUser(req.Steam64ID)
	s.issueSession(w, r, req.Steam64ID)
	writeJSON(w, 200, map[string]any{"user": userPublic(user)})
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, steamID string) {
	tok, err := auth.NewToken()
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	ttl := time.Duration(s.cfg.SessionTTLSeconds) * time.Second
	if err := s.store.CreateSession(tok, steamID, r.UserAgent(), ttl); err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	cookie := &http.Cookie{
		Name:     auth.CookieName,
		Value:    tok,
		Path:     "/",
		Expires:  time.Now().Add(ttl),
		MaxAge:   s.cfg.SessionTTLSeconds,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieName); err == nil {
		_ = s.store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
	})
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.FromContext(r.Context())
	if !ok || u == nil {
		writeErr(w, 401, "unauthorized", "not logged in")
		return
	}
	writeJSON(w, 200, map[string]any{"user": userPublic(u)})
}

func userPublic(u *store.User) map[string]any {
	return map[string]any{
		"steam64id":     u.Steam64ID,
		"display_name":  u.DisplayName,
		"is_owner":      u.IsOwner,
		"is_active":     u.IsActive,
		"must_reset_pw": u.MustResetPW,
	}
}

// ===== ingest =====

type ingestReq struct {
	RawText     string `json:"raw_text"`
	PlayerAlias string `json:"player_alias"`
	IsNewAlias  bool   `json:"is_new_alias"`
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	var req ingestReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "bad_request", "invalid json")
		return
	}
	if strings.TrimSpace(req.RawText) == "" {
		writeErr(w, 400, "bad_request", "raw_text empty")
		return
	}
	if strings.TrimSpace(req.PlayerAlias) == "" {
		writeErr(w, 400, "bad_request", "player_alias required")
		return
	}
	res, err := ingest.Ingest(r.Context(), s.store, req.RawText, u.Steam64ID, strings.TrimSpace(req.PlayerAlias), req.IsNewAlias)
	if err != nil {
		switch e := err.(type) {
		case *parser.Error:
			writeErr(w, 400, "parse_error", e.Error())
		case *ingest.ErrUnknownItem:
			writeJSON(w, 400, map[string]any{
				"error": map[string]any{
					"code":         "unknown_item",
					"message":      e.Error(),
					"unknown_items": e.Names,
				},
			})
		default:
			if errors.Is(err, ingest.ErrNewAliasRequired) {
				writeErr(w, 409, "new_alias_required", "该 player_alias 在当前 War 下不存在；勾选\"新建别名\"后重试")
				return
			}
			writeErr(w, 500, "internal", err.Error())
		}
		return
	}
	writeJSON(w, 200, res)
}

// ===== wars =====

func (s *Server) handleListWars(w http.ResponseWriter, r *http.Request) {
	wars, err := s.store.ListWars()
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"wars": warsPublic(wars)})
}

func (s *Server) handleActiveWar(w http.ResponseWriter, r *http.Request) {
	war, err := s.store.GetActiveWar()
	if err != nil {
		writeJSON(w, 200, map[string]any{"war": nil})
		return
	}
	writeJSON(w, 200, map[string]any{"war": warPublic(war)})
}

func warsPublic(ws []*store.War) []map[string]any {
	out := make([]map[string]any, 0, len(ws))
	for _, w := range ws {
		out = append(out, warPublic(w))
	}
	return out
}

func warPublic(w *store.War) map[string]any {
	endedAt := any(nil)
	if w.EndedAt.Valid {
		endedAt = w.EndedAt.Int64
	}
	return map[string]any{
		"id":         w.ID,
		"label":      w.Label,
		"started_at": w.StartedAt,
		"ended_at":   endedAt,
		"is_active":  w.IsActive,
		"notes":      w.Notes.String,
	}
}

// ===== stockpiles =====

func (s *Server) handleListStockpiles(w http.ResponseWriter, r *http.Request) {
	warID, err := strconv.ParseInt(r.URL.Query().Get("war_id"), 10, 64)
	if err != nil || warID == 0 {
		war, err := s.store.GetActiveWar()
		if err != nil {
			writeJSON(w, 200, map[string]any{"war": nil, "stockpiles": []any{}})
			return
		}
		warID = war.ID
	}
	list, err := s.store.ListStockpilesByWar(warID)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	rows := make([]map[string]any, 0, len(list))
	for _, sp := range list {
		rows = append(rows, map[string]any{
			"id":             sp.ID,
			"war_id":         sp.WarID,
			"region":         sp.Region,
			"sub_region":     sp.SubRegion,
			"building_name":  sp.BuildingName,
			"visibility":     sp.Visibility,
			"player_alias":   sp.PlayerAlias,
			"first_seen_at":  sp.FirstSeenAt,
			"last_seen_at":   sp.LastSeenAt,
			"last_uploader":  sp.LastUploaderName,
			"last_captured": sp.LastCapturedAt,
			"snapshot_id":    sp.SnapshotID,
			"item_count":     sp.ItemCount,
		})
	}
	writeJSON(w, 200, map[string]any{"war_id": warID, "stockpiles": rows})
}

func (s *Server) handleStockpileDetail(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "bad_request", "invalid id")
		return
	}
	sp, err := s.store.GetStockpile(id)
	if err != nil {
		writeErr(w, 404, "not_found", "stockpile not found")
		return
	}
	items, capturedAt, err := s.store.GetLatestItemsByStockpile(id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		rows = append(rows, map[string]any{
			"canonical_id": it.CanonicalID,
			"name_zh":      it.NameZH,
			"name_en":      nullStr(it.NameEN),
			"category":     nullStr(it.Category),
			"crate_size":   nullInt(it.CrateSize),
			"icon_source":  it.IconSource,
			"loose":        it.Loose,
			"crates":       it.Crates,
			"total":        it.Loose + it.Crates*nullableCrateSize(it.CrateSize),
		})
	}
	writeJSON(w, 200, map[string]any{
		"stockpile": map[string]any{
			"id":            sp.ID,
			"war_id":        sp.WarID,
			"region":        sp.Region,
			"sub_region":    sp.SubRegion,
			"building_name": sp.BuildingName,
			"visibility":    sp.Visibility,
			"player_alias":  sp.PlayerAlias,
			"first_seen_at": sp.FirstSeenAt,
			"last_seen_at":  sp.LastSeenAt,
			"captured_at":   capturedAt,
		},
		"items": rows,
	})
}

func nullableCrateSize(v sql.NullInt64) int64 {
	if v.Valid {
		return v.Int64
	}
	return 0
}

func (s *Server) handleStockpileTimeline(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "bad_request", "invalid id")
		return
	}
	cid := r.URL.Query().Get("item")
	if cid == "" {
		writeErr(w, 400, "bad_request", "item param required")
		return
	}
	loose, crates, err := s.store.GetTimeline(id, cid)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"stockpile_id": id,
		"canonical_id": cid,
		"series": []map[string]any{
			{"name": "散件", "data": pointsToArray(loose)},
			{"name": "箱装", "data": pointsToArray(crates)},
		},
	})
}

func pointsToArray(ps []store.TimelinePoint) [][]int64 {
	out := make([][]int64, 0, len(ps))
	for _, p := range ps {
		out = append(out, []int64{p.T * 1000, p.Q})
	}
	return out
}

// ===== items =====

func (s *Server) handleListItems(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListItems()
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": itemsPublic(items)})
}

func (s *Server) handleItemDetail(w http.ResponseWriter, r *http.Request) {
	cid := chi.URLParam(r, "cid")
	it, err := s.store.GetItem(cid)
	if err != nil {
		writeErr(w, 404, "not_found", "item not found")
		return
	}
	warID, _ := parseInt64(r.URL.Query().Get("war_id"))
	if warID == 0 {
		war, err := s.store.GetActiveWar()
		if err == nil {
			warID = war.ID
		}
	}
	var aggs []map[string]any
	if warID != 0 {
		list, err := s.store.GetItemAcrossStockpiles(warID, cid)
		if err == nil {
			for _, ag := range list {
				aggs = append(aggs, map[string]any{
					"stockpile_id":  ag.StockpileID,
					"region":        ag.Region,
					"building_name": ag.BuildingName,
					"player_alias":  ag.PlayerAlias,
					"loose":         ag.Loose,
					"crates":        ag.Crates,
					"captured_at":   ag.LastCapturedAt,
				})
			}
		}
	}
	writeJSON(w, 200, map[string]any{
		"item":         itemPublic(it),
		"war_id":       warID,
		"stockpiles":   aggs,
	})
}

func itemsPublic(items []*store.Item) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, itemPublic(it))
	}
	return out
}
func itemPublic(it *store.Item) map[string]any {
	return map[string]any{
		"canonical_id": it.CanonicalID,
		"name_zh":      it.NameZH,
		"name_en":      nullStr(it.NameEN),
		"faction":      nullStr(it.Faction),
		"category":     nullStr(it.Category),
		"crate_size":   nullInt(it.CrateSize),
		"icon_source":  it.IconSource,
		"icon_path":    nullStr(it.IconPath),
		"wiki_url":     nullStr(it.WikiURL),
	}
}

func nullStr(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}
func nullInt(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}

// ===== uploads =====

func (s *Server) handleRecentUploads(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n == 0 {
		n = 50
	}
	rows, err := s.store.RecentSnapshots(n)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, sn := range rows {
		out = append(out, map[string]any{
			"id":            sn.ID,
			"stockpile_id":  sn.StockpileID,
			"region":        sn.Region,
			"building_name": sn.BuildingName,
			"player_alias":  sn.PlayerAlias,
			"uploader_id":   sn.UploaderID,
			"uploader_name": sn.UploaderName,
			"captured_at":   sn.CapturedAt,
			"uploaded_at":   sn.UploadedAt,
			"item_count":    sn.ItemCount,
		})
	}
	writeJSON(w, 200, map[string]any{"uploads": out})
}

// ===== admin =====

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetStats()
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	dbStat, _ := os.Stat(s.cfg.DBPath)
	dbSize := int64(0)
	if dbStat != nil {
		dbSize = dbStat.Size()
	}
	writeJSON(w, 200, map[string]any{
		"user_count":          st.UserCount,
		"item_count":          st.ItemCount,
		"placeholder_items":   st.PlaceholderItems,
		"stockpile_count":     st.StockpileCount,
		"snapshot_count":      st.SnapshotCount,
		"active_war_label":    st.ActiveWarLabel,
		"active_war_started":  st.ActiveWarStartedAt,
		"db_path":             s.cfg.DBPath,
		"db_size_bytes":       dbSize,
	})
}

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	us, err := s.store.ListUsers()
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	out := make([]map[string]any, 0, len(us))
	for _, u := range us {
		out = append(out, map[string]any{
			"steam64id":     u.Steam64ID,
			"display_name":  u.DisplayName,
			"is_owner":      u.IsOwner,
			"is_active":     u.IsActive,
			"must_reset_pw": u.MustResetPW,
			"registered":    u.PasswordHash.Valid,
			"created_at":    u.CreatedAt,
			"registered_at": u.RegisteredAt.Int64,
		})
	}
	writeJSON(w, 200, map[string]any{"users": out})
}

type whitelistReq struct {
	Steam64ID   string `json:"steam64id"`
	DisplayName string `json:"display_name"`
}

func (s *Server) handleAdminAddWhitelist(w http.ResponseWriter, r *http.Request) {
	var req whitelistReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !validSteam64(req.Steam64ID) {
		writeErr(w, 400, "bad_request", "invalid steam64id")
		return
	}
	if err := s.store.AddWhitelist(req.Steam64ID, strings.TrimSpace(req.DisplayName)); err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminResetUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validSteam64(id) {
		writeErr(w, 400, "bad_request", "invalid id")
		return
	}
	if err := s.store.MarkResetPassword(id); err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminKickUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validSteam64(id) {
		writeErr(w, 400, "bad_request", "invalid id")
		return
	}
	cur, _ := auth.FromContext(r.Context())
	if cur != nil && cur.Steam64ID == id {
		writeErr(w, 400, "bad_request", "cannot kick yourself")
		return
	}
	if err := s.store.KickUser(id); err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

type newWarReq struct {
	Label string `json:"label"`
	Notes string `json:"notes"`
}

func (s *Server) handleAdminNewWar(w http.ResponseWriter, r *http.Request) {
	var req newWarReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Label) == "" {
		writeErr(w, 400, "bad_request", "label required")
		return
	}
	id, err := s.store.StartNewWar(strings.TrimSpace(req.Label), req.Notes)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"war_id": id})
}

func (s *Server) handleAdminEndWar(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "bad_request", "invalid id")
		return
	}
	if err := s.store.EndWar(id); err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

type deleteWarReq struct {
	ConfirmText string `json:"confirm_text"`
}

func (s *Server) handleAdminDeleteWar(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "bad_request", "invalid id")
		return
	}
	war, err := s.store.GetWar(id)
	if err != nil {
		writeErr(w, 404, "not_found", "war not found")
		return
	}
	if war.IsActive {
		writeErr(w, 400, "bad_request", "cannot delete active war")
		return
	}
	var req deleteWarReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	wanted := "DELETE WAR " + war.Label
	if req.ConfirmText != wanted {
		writeErr(w, 400, "bad_request", "confirm_text must equal "+wanted)
		return
	}
	if err := s.store.DeleteWar(id); err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminListItems(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListItems()
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": itemsPublic(items)})
}

type upsertItemReq struct {
	CanonicalID string   `json:"canonical_id"`
	NameZH      string   `json:"name_zh"`
	NameEN      string   `json:"name_en"`
	Faction     string   `json:"faction"`
	Category    string   `json:"category"`
	CrateSize   *int64   `json:"crate_size"`
	WikiURL     string   `json:"wiki_url"`
	Aliases     []string `json:"aliases"`
}

func (s *Server) handleAdminUpsertItem(w http.ResponseWriter, r *http.Request) {
	var req upsertItemReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		strings.TrimSpace(req.CanonicalID) == "" || strings.TrimSpace(req.NameZH) == "" {
		writeErr(w, 400, "bad_request", "canonical_id and name_zh required")
		return
	}
	it := &store.Item{
		CanonicalID: req.CanonicalID,
		NameZH:      req.NameZH,
		NameEN:      sql.NullString{String: req.NameEN, Valid: req.NameEN != ""},
		Faction:     sql.NullString{String: req.Faction, Valid: req.Faction != ""},
		Category:    sql.NullString{String: req.Category, Valid: req.Category != ""},
		WikiURL:     sql.NullString{String: req.WikiURL, Valid: req.WikiURL != ""},
		IconSource:  "placeholder",
	}
	if req.CrateSize != nil {
		it.CrateSize = sql.NullInt64{Int64: *req.CrateSize, Valid: true}
	}
	if err := s.store.UpsertItem(it, req.Aliases); err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminExportWar(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "bad_request", "invalid id")
		return
	}
	war, err := s.store.GetWar(id)
	if err != nil {
		writeErr(w, 404, "not_found", "war not found")
		return
	}
	stockpiles, _ := s.store.ListStockpilesByWar(id)
	type spOut struct {
		Stockpile any              `json:"stockpile"`
		Items     []map[string]any `json:"items"`
	}
	out := []spOut{}
	for _, sp := range stockpiles {
		items, _, _ := s.store.GetLatestItemsByStockpile(sp.ID)
		rows := make([]map[string]any, 0, len(items))
		for _, it := range items {
			rows = append(rows, map[string]any{
				"canonical_id": it.CanonicalID,
				"name_zh":      it.NameZH,
				"loose":        it.Loose,
				"crates":       it.Crates,
			})
		}
		out = append(out, spOut{
			Stockpile: map[string]any{
				"id":            sp.ID,
				"region":        sp.Region,
				"sub_region":    sp.SubRegion,
				"building_name": sp.BuildingName,
				"visibility":    sp.Visibility,
				"player_alias":  sp.PlayerAlias,
				"last_captured": sp.LastCapturedAt,
				"last_uploader": sp.LastUploaderName,
			},
			Items: rows,
		})
	}
	w.Header().Set("Content-Disposition", "attachment; filename=war-"+war.Label+".json")
	writeJSON(w, 200, map[string]any{
		"war":        warPublic(war),
		"stockpiles": out,
	})
}

// handleAdminUploadIcon 接收 multipart/form-data 上传 PNG，写入 assets/icons/{cid}.png 并更新 icon_source=manual。
func (s *Server) handleAdminUploadIcon(w http.ResponseWriter, r *http.Request) {
	cid := chi.URLParam(r, "cid")
	if cid == "" || strings.ContainsAny(cid, "/\\") {
		writeErr(w, 400, "bad_request", "invalid cid")
		return
	}
	if _, err := s.store.GetItem(cid); err != nil {
		writeErr(w, 404, "not_found", "item not found")
		return
	}
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		writeErr(w, 400, "bad_request", "expect multipart form (max 4MB)")
		return
	}
	file, hdr, err := r.FormFile("icon")
	if err != nil {
		writeErr(w, 400, "bad_request", "missing icon field")
		return
	}
	defer file.Close()
	if hdr.Size > 4<<20 {
		writeErr(w, 400, "bad_request", "icon too large (>4MB)")
		return
	}
	dir := filepath.Join(s.cfg.AssetsDir, "icons")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	dst := filepath.Join(dir, cid+".png")
	out, err := os.Create(dst)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	if err := s.store.UpdateItemIcon(cid, "icons/"+cid+".png", "manual"); err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok", "path": "icons/" + cid + ".png"})
}

func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" || strings.ContainsAny(name, "/\\") {
		writeErr(w, 400, "bad_request", "invalid icon name")
		return
	}
	path := filepath.Join(s.cfg.AssetsDir, "icons", name)
	if _, err := os.Stat(path); err == nil {
		http.ServeFile(w, r, path)
		return
	}
	// fallback unknown
	fb := filepath.Join(s.cfg.AssetsDir, "icons", "_unknown.png")
	if _, err := os.Stat(fb); err == nil {
		http.ServeFile(w, r, fb)
		return
	}
	// 极端 fallback：动态返回 1x1 透明 png
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(200)
	w.Write(unknownPNG)
}

// ===== SPA =====

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if s.web == nil {
		http.Error(w, "no web fs", 500)
		return
	}
	requestPath := strings.TrimPrefix(r.URL.Path, "/")
	if requestPath == "" {
		requestPath = "index.html"
	}
	// 尝试精确文件
	if f, err := s.web.Open(requestPath); err == nil {
		defer f.Close()
		stat, err := f.Stat()
		if err == nil && !stat.IsDir() {
			ct := guessMime(requestPath)
			if ct != "" {
				w.Header().Set("Content-Type", ct)
			}
			http.ServeContent(w, r, requestPath, stat.ModTime(), f.(interface {
				Read([]byte) (int, error)
				Seek(int64, int) (int64, error)
			}))
			return
		}
	}
	// fallback: index.html
	idx, err := s.web.Open("index.html")
	if err != nil {
		http.Error(w, "index.html missing", 500)
		return
	}
	defer idx.Close()
	stat, _ := idx.Stat()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if seeker, ok := idx.(interface {
		Read([]byte) (int, error)
		Seek(int64, int) (int64, error)
	}); ok && stat != nil {
		http.ServeContent(w, r, "index.html", stat.ModTime(), seeker)
		return
	}
	// 最后 fallback：直接复制
	buf := make([]byte, 0, 2048)
	tmp := make([]byte, 1024)
	for {
		n, err := idx.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	w.Write(buf)
}

func guessMime(p string) string {
	switch {
	case strings.HasSuffix(p, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(p, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(p, ".mjs"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(p, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(p, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(p, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(p, ".png"):
		return "image/png"
	case strings.HasSuffix(p, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(p, ".woff2"):
		return "font/woff2"
	}
	return ""
}

// 1x1 透明 PNG（fallback fallback）
var unknownPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}