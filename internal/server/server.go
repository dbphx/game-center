// Package server ghép các phần của game hub: API tài khoản / ví, danh sách game,
// WebSocket của từng game và file giao diện tĩnh.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/dbphx/game-center/internal/wallet"
)

// Game là một game trong hub. Thêm game mới = viết package trong games/ rồi đăng ký ở main.go.
type Game struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Players     string     `json:"players"` // ví dụ "2–4 người"
	Path        string     `json:"path"`    // trang của game, ví dụ "/tienlen/"
	Stats       func() any `json:"-"`       // số phòng / người đang chơi, hiện ở trang hub
	// WS là handler WebSocket của game, gắn ở /ws/<ID>.
	WS http.Handler `json:"-"`
}

type Server struct {
	wallet *wallet.Wallet
	games  []Game
	mux    *http.ServeMux
}

func New(w *wallet.Wallet, static fs.FS, games []Game) *Server {
	s := &Server{wallet: w, games: games, mux: http.NewServeMux()}
	s.mux.Handle("/", http.FileServer(http.FS(static)))
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	s.mux.HandleFunc("GET /api/games", s.listGames)
	s.mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]int{"startBalance": wallet.StartBalance, "dailyBonus": wallet.DailyBonus})
	})
	s.mux.HandleFunc("POST /api/guest", s.createGuest)
	s.mux.HandleFunc("GET /api/me", s.authed(s.me))
	s.mux.HandleFunc("POST /api/me", s.authed(s.rename))
	s.mux.HandleFunc("POST /api/daily", s.authed(s.daily))
	s.mux.HandleFunc("GET /api/history", s.authed(s.history))
	for _, g := range games {
		s.mux.Handle("/ws/"+g.ID, g.WS)
	}
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// UserByToken dùng cho các game xác thực kết nối WebSocket.
func (s *Server) UserByToken(ctx context.Context, token string) (wallet.User, error) {
	return s.wallet.UserByToken(ctx, token)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) authed(h func(http.ResponseWriter, *http.Request, wallet.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		u, err := s.wallet.UserByToken(r.Context(), token)
		if errors.Is(err, wallet.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "phiên đăng nhập không hợp lệ")
			return
		}
		if err != nil {
			s.internal(w, err)
			return
		}
		h(w, r, u)
	}
}

func (s *Server) internal(w http.ResponseWriter, err error) {
	log.Printf("api: %v", err)
	writeError(w, http.StatusInternalServerError, "lỗi máy chủ, thử lại sau")
}

func readName(w http.ResponseWriter, r *http.Request) string {
	var body struct {
		Name string `json:"name"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body)
	return body.Name
}

func (s *Server) listGames(w http.ResponseWriter, _ *http.Request) {
	type gameView struct {
		Game
		Online any `json:"online,omitempty"`
	}
	out := make([]gameView, len(s.games))
	for i, g := range s.games {
		out[i] = gameView{Game: g}
		if g.Stats != nil {
			out[i].Online = g.Stats()
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createGuest(w http.ResponseWriter, r *http.Request) {
	name := readName(w, r)
	if strings.TrimSpace(name) == "" {
		writeError(w, http.StatusBadRequest, "nhập tên trước đã")
		return
	}
	u, token, err := s.wallet.CreateGuest(r.Context(), name)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": u})
}

func (s *Server) me(w http.ResponseWriter, _ *http.Request, u wallet.User) {
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) rename(w http.ResponseWriter, r *http.Request, u wallet.User) {
	name := readName(w, r)
	if strings.TrimSpace(name) == "" {
		writeError(w, http.StatusBadRequest, "tên không được để trống")
		return
	}
	u, err := s.wallet.Rename(r.Context(), u.ID, name)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) daily(w http.ResponseWriter, r *http.Request, u wallet.User) {
	u, err := s.wallet.ClaimDaily(r.Context(), u.ID)
	if errors.Is(err, wallet.ErrAlreadyClaimed) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request, u wallet.User) {
	entries, err := s.wallet.History(r.Context(), u.ID, 30)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}
