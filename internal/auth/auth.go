// Package auth 实现 argon2id 哈希、随机 session token 与 chi 中间件。
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/Jianbao233/foxhole-quartermaster/internal/store"
)

const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16

	CookieName = "foxhole_sid"
)

// Hash argon2id 编码：$argon2id$v=19$m=...,t=...,p=...$<salt_b64>$<hash_b64>
func Hash(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func Verify(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var (
		version int
		m, t    uint32
		p       uint8
	)
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(want, got) == 1
}

// NewToken 32 字节随机 hex。
func NewToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxSession
)

// FromContext 从 ctx 取当前用户。
func FromContext(ctx context.Context) (*store.User, bool) {
	u, ok := ctx.Value(ctxUser).(*store.User)
	return u, ok
}
func SessionFromContext(ctx context.Context) (*store.Session, bool) {
	s, ok := ctx.Value(ctxSession).(*store.Session)
	return s, ok
}

// Middleware 解析 cookie 并把 user 注入 context（不强制登录）。
func Middleware(st *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			sess, user, err := st.ResolveSession(cookie.Value)
			if err != nil || sess == nil || user == nil || !user.IsActive {
				next.ServeHTTP(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), ctxUser, user)
			ctx = context.WithValue(ctx, ctxSession, sess)
			st.TouchSession(sess.Token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireMember 强制登录用户。
func RequireMember(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := FromContext(r.Context())
		if !ok || u == nil {
			http.Error(w, `{"error":{"code":"unauthorized","message":"login required"}}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireOwner 强制 owner。
func RequireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := FromContext(r.Context())
		if !ok || u == nil {
			http.Error(w, `{"error":{"code":"unauthorized","message":"login required"}}`, http.StatusUnauthorized)
			return
		}
		if !u.IsOwner {
			http.Error(w, `{"error":{"code":"forbidden","message":"owner only"}}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var ErrNotFound = errors.New("not found")