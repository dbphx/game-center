package baucua

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/dbphx/game-center/internal/wallet"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := errStatus(err)
	msg := err.Error()
	switch status {
	case http.StatusInternalServerError:
		log.Printf("baucua: %v", err)
		msg = "lỗi máy chủ, thử lại sau"
	case http.StatusPaymentRequired:
		msg = "không đủ xu"
	case http.StatusUnauthorized:
		msg = "phiên đăng nhập không hợp lệ"
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func bearer(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// Handler trả API bầu cua, gắn ở /api/baucua/... Xem trạng thái không cần đăng nhập; đặt / huỷ cược thì cần
// đăng nhập và đủ máu vào Casino.
func (g *Game) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/baucua/state", func(w http.ResponseWriter, r *http.Request) {
		var uid int64
		if tok := bearer(r); tok != "" {
			// Token hỏng thì vẫn cho xem như khách.
			if u, err := g.w.UserByToken(r.Context(), tok); err == nil {
				uid = u.ID
			} else if !errors.Is(err, wallet.ErrNotFound) {
				writeErr(w, err)
				return
			}
		}
		s, err := g.State(r.Context(), uid)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, s)
	})

	type body struct {
		Face   string `json:"face"`
		Amount int64  `json:"amount"`
	}
	// post bọc xác thực, kiểm máu và đọc body cho các thao tác đặt / huỷ cược.
	post := func(pattern string, op func(r *http.Request, uid int64, b body) (State, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			u, err := g.w.UserByToken(r.Context(), bearer(r))
			if err != nil {
				writeErr(w, err)
				return
			}
			if err := wallet.CheckZone(u, "casino"); err != nil {
				writeErr(w, err)
				return
			}
			var b body
			json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b)
			s, err := op(r, u.ID, b)
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, s)
		})
	}
	post("POST /api/baucua/bet", func(r *http.Request, uid int64, b body) (State, error) {
		return g.Bet(r.Context(), uid, b.Face, b.Amount)
	})
	post("POST /api/baucua/clear", func(r *http.Request, uid int64, _ body) (State, error) {
		return g.Clear(r.Context(), uid)
	})
	return mux
}
