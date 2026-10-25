package farm

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
		log.Printf("farm: %v", err)
		msg = "lỗi máy chủ, thử lại sau"
	case http.StatusPaymentRequired:
		msg = "không đủ xu"
	case http.StatusUnauthorized:
		msg = "phiên đăng nhập không hợp lệ"
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

// Handler trả API nông trại, gắn ở /api/farm và /api/farm/... (cần đăng nhập, trừ catalog).
func (f *Farm) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/farm/catalog", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, Catalog()) })

	// reply trả nông trại mới kèm user (số dư, level) để giao diện cập nhật một lần.
	reply := func(w http.ResponseWriter, r *http.Request, uid int64, s State, g *Gained) {
		u, err := f.w.User(r.Context(), uid)
		if err != nil {
			writeErr(w, err)
			return
		}
		out := map[string]any{"farm": s, "user": u}
		if g != nil {
			out["gained"] = g
		}
		writeJSON(w, http.StatusOK, out)
	}
	type body struct {
		Plot *int   `json:"plot"`
		Crop string `json:"crop"`
		Kind string `json:"kind"`
		ID   int64  `json:"id"`
		Qty  int    `json:"qty"`
		Item string `json:"item"`
		Feed string `json:"feed"`
	}
	// handle bọc xác thực, đọc body và gọi thao tác; thao tác trả State và (tuỳ chọn) phần thưởng.
	handle := func(pattern string, op func(r *http.Request, uid int64, b body) (State, *Gained, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			u, err := f.w.UserByToken(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
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
				// Làm nông cần đủ máu và không nằm viện; xem nông trại (GET) thì luôn được.
				if err := wallet.CheckZone(u, GameID); err != nil {
					writeErr(w, err)
					return
				}
				json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b)
			}
			s, g, err := op(r, u.ID, b)
			if err != nil {
				writeErr(w, err)
				return
			}
			reply(w, r, u.ID, s, g)
		})
	}
	plot := func(b body) int {
		if b.Plot == nil {
			return -1
		}
		return *b.Plot
	}
	noGain := func(s State, err error) (State, *Gained, error) { return s, nil, err }
	gain := func(s State, g Gained, err error) (State, *Gained, error) { return s, &g, err }

	handle("GET /api/farm", func(r *http.Request, uid int64, _ body) (State, *Gained, error) {
		return noGain(f.State(r.Context(), uid))
	})
	handle("POST /api/farm/plots/buy", func(r *http.Request, uid int64, _ body) (State, *Gained, error) {
		return noGain(f.BuyPlot(r.Context(), uid))
	})
	handle("POST /api/farm/plant", func(r *http.Request, uid int64, b body) (State, *Gained, error) {
		return noGain(f.Plant(r.Context(), uid, plot(b), b.Crop))
	})
	handle("POST /api/farm/harvest", func(r *http.Request, uid int64, b body) (State, *Gained, error) {
		return gain(f.Harvest(r.Context(), uid, plot(b)))
	})
	handle("POST /api/farm/harvest-all", func(r *http.Request, uid int64, _ body) (State, *Gained, error) {
		return gain(f.HarvestAll(r.Context(), uid))
	})
	// Chuồng (/animals), sân gia cầm (/poultry, có sẵn nên không có /yard/buy), ao (/fish).
	for _, place := range []string{placeBarn, placeYard, placePond} {
		item := map[string]string{placeBarn: "animals", placeYard: "poultry", placePond: "fish"}[place]
		if place != placeYard {
			handle("POST /api/farm/"+place+"/buy", func(r *http.Request, uid int64, _ body) (State, *Gained, error) {
				return noGain(f.BuyBuilding(r.Context(), uid, place))
			})
		}
		handle("POST /api/farm/"+place+"/expand", func(r *http.Request, uid int64, _ body) (State, *Gained, error) {
			return noGain(f.ExpandBuilding(r.Context(), uid, place))
		})
		handle("POST /api/farm/"+item+"/buy", func(r *http.Request, uid int64, b body) (State, *Gained, error) {
			return noGain(f.BuyStock(r.Context(), uid, place, b.Kind))
		})
		handle("POST /api/farm/"+item+"/sell", func(r *http.Request, uid int64, b body) (State, *Gained, error) {
			return gain(f.SellStock(r.Context(), uid, place, b.ID))
		})
		handle("POST /api/farm/"+item+"/feed", func(r *http.Request, uid int64, b body) (State, *Gained, error) {
			return gain(f.FeedStock(r.Context(), uid, place, b.ID))
		})
		handle("POST /api/farm/"+item+"/collect", func(r *http.Request, uid int64, b body) (State, *Gained, error) {
			return gain(f.CollectStock(r.Context(), uid, place, b.ID))
		})
		if place != placePond {
			handle("POST /api/farm/"+item+"/produce", func(r *http.Request, uid int64, b body) (State, *Gained, error) {
				return gain(f.ProduceStock(r.Context(), uid, place, b.ID))
			})
		}
	}

	// Làm nhanh hàng loạt.
	handle("POST /api/farm/plant-all", func(r *http.Request, uid int64, b body) (State, *Gained, error) {
		return gain(f.PlantAll(r.Context(), uid, b.Crop))
	})
	handle("POST /api/farm/feed-all", func(r *http.Request, uid int64, _ body) (State, *Gained, error) {
		return gain(f.FeedAll(r.Context(), uid))
	})
	handle("POST /api/farm/produce-all", func(r *http.Request, uid int64, _ body) (State, *Gained, error) {
		return gain(f.ProduceAll(r.Context(), uid))
	})
	handle("POST /api/farm/fish/collect-all", func(r *http.Request, uid int64, _ body) (State, *Gained, error) {
		return gain(f.CollectAllFish(r.Context(), uid))
	})

	// Kho giống và kho sản phẩm.
	handle("POST /api/farm/feeds/buy", func(r *http.Request, uid int64, b body) (State, *Gained, error) {
		return noGain(f.BuyFeeds(r.Context(), uid, b.Feed, b.Qty))
	})
	handle("POST /api/farm/seeds/buy", func(r *http.Request, uid int64, b body) (State, *Gained, error) {
		return noGain(f.BuySeeds(r.Context(), uid, b.Crop, b.Qty))
	})
	handle("POST /api/farm/products/sell", func(r *http.Request, uid int64, b body) (State, *Gained, error) {
		return gain(f.SellProduct(r.Context(), uid, b.Item, b.Qty))
	})
	handle("POST /api/farm/products/sell-all", func(r *http.Request, uid int64, _ body) (State, *Gained, error) {
		return gain(f.SellAllProducts(r.Context(), uid))
	})
	for _, store := range []string{storeSeeds, storeProducts} {
		handle("POST /api/farm/storage/"+store+"/expand", func(r *http.Request, uid int64, _ body) (State, *Gained, error) {
			return noGain(f.ExpandStore(r.Context(), uid, store))
		})
	}
	return mux
}
