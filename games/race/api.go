package race

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/dbphx/game-center/internal/wallet"
)

// Game gộp garage (API HTTP /api/race) và các phòng đua (WebSocket /ws/race).
type Game struct {
	Garage *Garage
	Hub    *Hub
}

// New dựng Trường đua. tracks là thư mục chứa <id>.json (vd fs.Sub(public, "public/race/tracks"));
// nil hoặc chưa có đường thì vẫn chạy được, chỉ không tạo được phòng.
func New(ctx context.Context, w *wallet.Wallet, tracks fs.FS, opts Options) (*Game, error) {
	g, err := NewGarage(ctx, w)
	if err != nil {
		return nil, err
	}
	ts, err := LoadTracks(tracks)
	if err != nil {
		return nil, fmt.Errorf("đọc đường đua: %w", err)
	}
	if len(ts) == 0 {
		log.Printf("race: chưa có đường đua nào, Trường đua chưa tạo được phòng")
	}
	return &Game{Garage: g, Hub: NewHub(w, g, ts, opts)}, nil
}

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
		log.Printf("race: %v", err)
		msg = "lỗi máy chủ, thử lại sau"
	case http.StatusPaymentRequired:
		msg = "không đủ xu"
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

// Handler trả API garage, gắn ở /api/race/... (cần đăng nhập).
//
//	GET  /api/race/garage                               → garage
//	POST /api/race/car/buy {car}, /car/select {car}
//	POST /api/race/upgrade {car, part}                  part: engine / turbo / tires
//	POST /api/race/item/buy {item}
//	POST /api/race/equip {car, kind, item}              kind: spoiler / neon / paint / decal; item "" = tháo
//
// Các POST trả {garage, user}.
func (g *Garage) Handler() http.Handler {
	mux := http.NewServeMux()
	type body struct {
		Car  string `json:"car"`
		Part string `json:"part"`
		Item string `json:"item"`
		Kind string `json:"kind"`
	}
	auth := func(w http.ResponseWriter, r *http.Request) (wallet.User, bool) {
		u, err := g.w.UserByToken(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if errors.Is(err, wallet.ErrNotFound) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "phiên đăng nhập không hợp lệ"})
			return u, false
		}
		if err != nil {
			writeErr(w, err)
			return u, false
		}
		return u, true
	}
	mux.HandleFunc("GET /api/race/garage", func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth(w, r)
		if !ok {
			return
		}
		v, err := g.View(r.Context(), u.ID)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})
	post := func(pattern string, op func(ctx context.Context, uid int64, b body) (GarageView, error)) {
		mux.HandleFunc("POST "+pattern, func(w http.ResponseWriter, r *http.Request) {
			u, ok := auth(w, r)
			if !ok {
				return
			}
			// Mua sắm cần đủ máu vào khu, như các khu khác.
			if err := wallet.CheckZone(u, GameID); err != nil {
				writeErr(w, err)
				return
			}
			var b body
			json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b)
			v, err := op(r.Context(), u.ID, b)
			if err != nil {
				writeErr(w, err)
				return
			}
			user, err := g.w.User(r.Context(), u.ID)
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"garage": v, "user": user})
		})
	}
	post("/api/race/car/buy", func(ctx context.Context, uid int64, b body) (GarageView, error) { return g.BuyCar(ctx, uid, b.Car) })
	post("/api/race/car/select", func(ctx context.Context, uid int64, b body) (GarageView, error) {
		return g.SelectCar(ctx, uid, b.Car)
	})
	post("/api/race/upgrade", func(ctx context.Context, uid int64, b body) (GarageView, error) {
		return g.Upgrade(ctx, uid, b.Car, b.Part)
	})
	post("/api/race/item/buy", func(ctx context.Context, uid int64, b body) (GarageView, error) { return g.BuyItem(ctx, uid, b.Item) })
	post("/api/race/equip", func(ctx context.Context, uid int64, b body) (GarageView, error) {
		return g.Equip(ctx, uid, b.Car, b.Kind, b.Item)
	})
	return mux
}
