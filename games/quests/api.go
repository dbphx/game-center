package quests

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
		log.Printf("quests: %v", err)
		msg = "lỗi máy chủ, thử lại sau"
	case http.StatusPaymentRequired:
		msg = "không đủ xu"
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

// Handler trả API nhiệm vụ, gắn ở /api/quests (cần đăng nhập).
//
//	GET  /api/quests                    → trạng thái (State)
//	POST /api/quests/claim {id}         → {quests, gained}
//	POST /api/quests/chest {kind}       → {quests, gained}       kind: daily / weekly
//	POST /api/quests/reroll {id}        → {quests, cost}
//	POST /api/quests/assemble {item}    → {quests, granted}
//	POST /api/quests/exchange {tier}    → {quests, gained}
func (q *Quests) Handler() http.Handler {
	mux := http.NewServeMux()
	type body struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
		Item string `json:"item"`
		Tier string `json:"tier"`
	}
	route := func(pattern string, op func(r *http.Request, uid int64, b body) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			u, err := q.w.UserByToken(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			if errors.Is(err, wallet.ErrNotFound) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "phiên đăng nhập không hợp lệ"})
				return
			}
			if err != nil {
				writeErr(w, err)
				return
			}
			var b body
			if r.Method == http.MethodPost {
				json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b)
			}
			out, err := op(r, u.ID, b)
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, out)
		})
	}
	route("GET /api/quests", func(r *http.Request, uid int64, _ body) (any, error) {
		return q.View(r.Context(), uid)
	})
	route("POST /api/quests/claim", func(r *http.Request, uid int64, b body) (any, error) {
		s, g, err := q.Claim(r.Context(), uid, b.ID)
		return map[string]any{"quests": s, "gained": g}, err
	})
	route("POST /api/quests/chest", func(r *http.Request, uid int64, b body) (any, error) {
		s, g, err := q.Chest(r.Context(), uid, b.Kind)
		return map[string]any{"quests": s, "gained": g}, err
	})
	route("POST /api/quests/reroll", func(r *http.Request, uid int64, b body) (any, error) {
		s, cost, err := q.Reroll(r.Context(), uid, b.ID)
		return map[string]any{"quests": s, "cost": cost}, err
	})
	route("POST /api/quests/assemble", func(r *http.Request, uid int64, b body) (any, error) {
		s, g, err := q.Assemble(r.Context(), uid, b.Item)
		return map[string]any{"quests": s, "granted": g}, err
	})
	route("POST /api/quests/exchange", func(r *http.Request, uid int64, b body) (any, error) {
		s, g, err := q.Exchange(r.Context(), uid, b.Tier)
		return map[string]any{"quests": s, "gained": g}, err
	})
	return mux
}
