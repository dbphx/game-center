package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/dbphx/game-center/games/farm"
	"github.com/dbphx/game-center/internal/wallet"
)

// Nông trại không dùng WebSocket: chỉ có API HTTP gắn ở /api/farm.
func newFarmServer(t *testing.T) (*httptest.Server, *wallet.Wallet) {
	t.Helper()
	w, err := wallet.Open(filepath.Join(t.TempDir(), "farm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	f, err := farm.New(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(w, fstest.MapFS{}, []Game{{ID: farm.GameID, Name: "Nông trại", Path: "/farm/", API: f.Handler()}}))
	t.Cleanup(srv.Close)
	return srv, w
}

type farmReply struct {
	Farm   farm.State   `json:"farm"`
	User   wallet.User  `json:"user"`
	Gained *farm.Gained `json:"gained"`
	Error  string       `json:"error"`
}

func TestFarmAPI(t *testing.T) {
	srv, w := newFarmServer(t)
	a := newAccount(t, srv, "nongdan")

	var cat map[string]any
	if code := call(t, srv, "GET", "/api/farm/catalog", "", nil, &cat); code != 200 || cat["crops"] == nil || cat["barn"] == nil || cat["plots"] == nil {
		t.Fatalf("catalog: %d %v", code, cat)
	}
	if code := call(t, srv, "GET", "/api/farm", "", nil, nil); code != 401 {
		t.Fatalf("chưa đăng nhập phải 401, được %d", code)
	}
	if code := call(t, srv, "GET", "/ws/farm", "", nil, nil); code != 404 {
		t.Fatalf("nông trại không có WebSocket, /ws/farm phải 404, được %d", code)
	}

	var r farmReply
	if code := call(t, srv, "GET", "/api/farm", a.Token, nil, &r); code != 200 || len(r.Farm.Plots) != 6 || r.Farm.NextPlotPrice != 10000 || r.User.ID != a.User.ID {
		t.Fatalf("nông trại mới: %d %+v", code, r)
	}
	if code := call(t, srv, "POST", "/api/farm/plant", a.Token, map[string]any{"plot": 0, "crop": "carrot"}, &r); code != 200 ||
		r.Farm.Plots[0].Crop != "carrot" || r.User.Balance != wallet.StartBalance-50 {
		t.Fatalf("trồng: %d %+v", code, r)
	}

	cases := []struct {
		path string
		body any
		want int
	}{
		{"/api/farm/plant", map[string]any{"plot": 0, "crop": "corn"}, 409},   // ô đang có cây
		{"/api/farm/plant", map[string]any{"plot": 50, "crop": "corn"}, 400},  // ô không tồn tại
		{"/api/farm/plant", map[string]any{"crop": "corn"}, 400},              // thiếu ô
		{"/api/farm/plant", map[string]any{"plot": 1, "crop": "tomato"}, 403}, // cần level 2
		{"/api/farm/harvest", map[string]any{"plot": 0}, 409},                 // chưa chín
		{"/api/farm/harvest-all", nil, 409},                                   // chưa có ô nào chín
		{"/api/farm/plots/buy", nil, 402},                                     // 10.000 xu > số dư
		{"/api/farm/barn/buy", nil, 402},                                      // chuồng Lv1 nhưng 3.000 xu > số dư
		{"/api/farm/animals/buy", map[string]any{"kind": "pig"}, 409},         // chưa có chuồng
		{"/api/farm/fish/buy", map[string]any{"kind": "tilapia"}, 409},        // chưa có ao
		{"/api/farm/fish/sell", map[string]any{"id": 1}, 409},                 // chưa có ao
		{"/api/farm/fish/feed", map[string]any{"id": 1}, 409},                 // chưa có ao
	}
	for _, tc := range cases {
		var e farmReply
		if code := call(t, srv, "POST", tc.path, a.Token, tc.body, &e); code != tc.want || e.Error == "" {
			t.Errorf("%s %v: muốn %d có thông báo lỗi, được %d %q", tc.path, tc.body, tc.want, code, e.Error)
		}
	}

	// Ao cá cần level 1: nạp thêm xu rồi mua ao, thả cá rô phi, bán cá chưa lớn bị từ chối.
	if _, err := w.Settle(context.Background(), "test", "nạp xu", map[int64]int64{a.User.ID: 5000}); err != nil {
		t.Fatal(err)
	}
	if code := call(t, srv, "POST", "/api/farm/pond/buy", a.Token, nil, &r); code != 200 || r.Farm.Pond == nil || r.Farm.Pond.Capacity != 5 {
		t.Fatalf("mua ao: %d %+v", code, r.Farm.Pond)
	}
	if code := call(t, srv, "POST", "/api/farm/pond/buy", a.Token, nil, nil); code != 409 {
		t.Fatalf("mua ao lần hai phải 409, được %d", code)
	}
	if code := call(t, srv, "POST", "/api/farm/fish/buy", a.Token, map[string]any{"kind": "tilapia"}, &r); code != 200 || r.Farm.Pond.Used != 1 {
		t.Fatalf("thả cá: %d %+v", code, r.Farm.Pond)
	}
	fishID := r.Farm.Pond.Fish[0].ID
	if code := call(t, srv, "POST", "/api/farm/fish/sell", a.Token, map[string]any{"id": fishID}, nil); code != 409 {
		t.Fatalf("bán cá chưa lớn phải 409, được %d", code)
	}
	if code := call(t, srv, "POST", "/api/farm/fish/sell", a.Token, map[string]any{"id": fishID + 100}, nil); code != 404 {
		t.Fatalf("bán cá không có phải 404, được %d", code)
	}
	if code := call(t, srv, "POST", "/api/farm/fish/buy", a.Token, map[string]any{"kind": "shark"}, nil); code != 400 {
		t.Fatalf("cá lạ phải 400, được %d", code)
	}

	var hist []wallet.Entry
	call(t, srv, "GET", "/api/history", a.Token, nil, &hist)
	if len(hist) < 3 || hist[0].Game != farm.GameID || hist[0].Note != "Mua Cá rô phi" || hist[1].Note != "Mua ao cá" {
		t.Fatalf("sổ cái: %+v", hist)
	}
}

func TestFarmListedInGames(t *testing.T) {
	srv, _ := newFarmServer(t)
	var games []map[string]any
	if code := call(t, srv, "GET", "/api/games", "", nil, &games); code != http.StatusOK || len(games) != 1 || games[0]["path"] != "/farm/" {
		t.Fatalf("danh sách game: %d %+v", code, games)
	}
}
