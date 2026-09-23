package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionTTL = 30 * 24 * time.Hour

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_.]{3,32}$`)

type Handler struct {
	DB *pgxpool.Pool
	// dummyHash is verified against when a login names an unknown account,
	// so response time does not reveal whether the account exists.
	dummyHash string
}

func NewHandler(db *pgxpool.Pool) (*Handler, error) {
	dummy, err := HashPassword("astelyn-dummy-password")
	if err != nil {
		return nil, err
	}
	return &Handler{DB: db, dummyHash: dummy}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/auth/register", h.register)
	mux.HandleFunc("POST /v1/auth/login", h.login)
	mux.HandleFunc("POST /v1/auth/logout", h.logout)
	mux.HandleFunc("GET /v1/auth/me", h.me)
}

type user struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type authResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      user      `json:"user"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)

	if !usernameRe.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "username must be 3-32 characters: letters, numbers, _ or .")
		return
	}
	if addr, err := mail.ParseAddress(req.Email); err != nil || addr.Address != req.Email {
		writeError(w, http.StatusBadRequest, "invalid email address")
		return
	}
	if n := len(req.Password); n < 8 || n > 128 {
		writeError(w, http.StatusBadRequest, "password must be 8-128 characters")
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	u := user{Username: req.Username, Email: req.Email}
	err = h.DB.QueryRow(r.Context(),
		`INSERT INTO users (username, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		u.Username, u.Email, hash,
	).Scan(&u.ID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "username or email already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.issueSession(w, r.Context(), u, http.StatusCreated)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Identifier string `json:"identifier"` // username or email
		Password   string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}

	var u user
	var hash string
	err := h.DB.QueryRow(r.Context(),
		`SELECT id, username, email, password_hash FROM users
		 WHERE lower(username) = lower($1) OR lower(email) = lower($1)`,
		strings.TrimSpace(req.Identifier),
	).Scan(&u.ID, &u.Username, &u.Email, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		VerifyPassword(req.Password, h.dummyHash)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	ok, err := VerifyPassword(req.Password, hash)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	h.issueSession(w, r.Context(), u, http.StatusOK)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing token")
		return
	}
	h.DB.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash = $1`, hashToken(token))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing token")
		return
	}
	var u user
	err := h.DB.QueryRow(r.Context(),
		`SELECT u.id, u.username, u.email
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.expires_at > now()`,
		hashToken(token),
	).Scan(&u.ID, &u.Username, &u.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *Handler) issueSession(w http.ResponseWriter, ctx context.Context, u user, status int) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().Add(sessionTTL)

	if _, err := h.DB.Exec(ctx,
		`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		u.ID, hashToken(token), expires,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, status, authResponse{Token: token, ExpiresAt: expires, User: u})
}

// Only a SHA-256 of the token is stored, so a database leak does not yield usable sessions.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return h[len(prefix):], true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
