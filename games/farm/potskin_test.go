package farm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dbphx/game-center/internal/wallet"
)

func TestPotSkinCatalog(t *testing.T) {
	want := []struct {
		id    string
		price int64
	}{{"default", 0}, {"white", 0}, {"flower", 20_000}, {"wood", 20_000}, {"bluewhite", 50_000},
		{"jade", 100_000}, {"cloud", 200_000}, {"crystal", 300_000}, {"gold", 500_000}}
	if len(PotSkins) != len(want) {
		t.Fatalf("số kiểu chậu: %d", len(PotSkins))
	}
	for i, w := range want {
		if PotSkins[i].ID != w.id || PotSkins[i].Price != w.price || PotSkins[i].Name == "" {
			t.Fatalf("kiểu %d: %+v", i, PotSkins[i])
		}
	}
	if _, ok := Catalog()["potSkins"]; !ok {
		t.Fatal("catalog thiếu potSkins")
	}
}

func TestPotSkinBuyAndSet(t *testing.T) {
	e := newEnv(t, 1_000_000, 0)
	e.openFloors(1)
	s := e.ok(e.f.BuyPot(ctx, e.uid, 1))
	if got := s.Cloud.Skins; len(got) != 2 || got[0] != "default" || got[1] != "white" {
		t.Fatalf("kiểu có sẵn: %v", got)
	}
	if p, _ := s.Cloud.pot(1, 0); p.Skin != "default" {
		t.Fatalf("chậu mới phải dùng kiểu mặc định: %+v", p)
	}
	// Kiểu miễn phí lắp được ngay; kiểu chưa mua thì không.
	s = e.ok(e.f.SetPotSkin(ctx, e.uid, 1, 0, "white"))
	if p, _ := s.Cloud.pot(1, 0); p.Skin != "white" {
		t.Fatalf("lắp gốm trắng: %+v", p)
	}
	_, err := e.f.SetPotSkin(ctx, e.uid, 1, 0, "gold")
	wantStatus(t, err, http.StatusConflict)

	before := e.user().Balance
	s = e.ok(e.f.BuyPotSkin(ctx, e.uid, "jade"))
	if got := before - e.user().Balance; got != 100_000 {
		t.Fatalf("giá ngọc lục bảo: %d", got)
	}
	if !contains(s.Cloud.Skins, "jade") {
		t.Fatalf("chưa có jade: %v", s.Cloud.Skins)
	}
	_, err = e.f.BuyPotSkin(ctx, e.uid, "jade")
	wantStatus(t, err, http.StatusConflict)
	_, err = e.f.BuyPotSkin(ctx, e.uid, "white")
	wantStatus(t, err, http.StatusConflict)
	_, err = e.f.BuyPotSkin(ctx, e.uid, "nope")
	wantStatus(t, err, http.StatusNotFound)

	// Đổi kiểu không đụng tới cây đang trồng.
	s = e.ok(e.f.PlantPot(ctx, e.uid, 1, 0))
	p0, _ := s.Cloud.pot(1, 0)
	s = e.ok(e.f.SetPotSkin(ctx, e.uid, 1, 0, "jade"))
	if p, _ := s.Cloud.pot(1, 0); p.Skin != "jade" || p.PlantedAt != p0.PlantedAt || p.ReadyAt != p0.ReadyAt {
		t.Fatalf("đổi kiểu làm đổi cây: %+v / %+v", p0, p)
	}
	s = e.ok(e.f.SetPotSkin(ctx, e.uid, 1, 0, "default"))
	if p, _ := s.Cloud.pot(1, 0); p.Skin != "default" {
		t.Fatalf("về mặc định: %+v", p)
	}

	// Chỗ sai, chưa có chậu.
	_, err = e.f.SetPotSkin(ctx, e.uid, 1, 9, "white")
	wantStatus(t, err, http.StatusBadRequest)
	_, err = e.f.SetPotSkin(ctx, e.uid, 1, 3, "white")
	wantStatus(t, err, http.StatusConflict)
	_, err = e.f.SetPotSkin(ctx, e.uid, 1, 0, "nope")
	wantStatus(t, err, http.StatusConflict)
}

func TestPotSkinNotEnoughCoins(t *testing.T) {
	e := newEnv(t, 10_000, 0)
	_, err := e.f.BuyPotSkin(ctx, e.uid, "gold")
	wantStatus(t, err, http.StatusPaymentRequired)
	if b := e.user().Balance; b != 10_000 {
		t.Fatalf("thiếu xu mà vẫn trừ: %d", b)
	}
}

func TestPotSkinWorth(t *testing.T) {
	e := newEnv(t, 1_000_000, 0)
	before := e.worth()
	e.ok(e.f.BuyPotSkin(ctx, e.uid, "crystal"))
	if got := e.worth() - before; got != 300_000 {
		t.Fatalf("giá trị tăng %d, muốn 300.000", got)
	}
}

func TestPotSkinColumnUpgrade(t *testing.T) {
	e := newEnv(t, 1_000_000, 0)
	e.openFloors(1)
	e.ok(e.f.BuyPot(ctx, e.uid, 1))
	e.exec(`ALTER TABLE farm_pots DROP COLUMN skin`)
	for range 2 { // khởi động lại hai lần: lần sau không lỗi
		if _, err := New(ctx, e.w); err != nil {
			t.Fatal(err)
		}
	}
	s := e.ok(e.f.State(ctx, e.uid))
	if p, ok := s.Cloud.pot(1, 0); !ok || p.Skin != "default" {
		t.Fatalf("chậu cũ sau nâng cấp: %+v %v", p, ok)
	}
}

func TestPotSkinAPI(t *testing.T) {
	e := newEnv(t, 0, 0)
	u, token, err := e.w.Register(ctx, "chaudep", "matkhau123", wallet.Profile{Name: "Chậu"})
	if err != nil {
		t.Fatal(err)
	}
	e.uid = u.ID
	e.setBalance(u.ID, 100_000_000)
	e.openFloors(1)
	e.ok(e.f.BuyPot(ctx, u.ID, 1))
	srv := httptest.NewServer(e.f.Handler())
	defer srv.Close()
	post := func(path, body string) (int, map[string]json.RawMessage) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]json.RawMessage
		json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	if code, out := post("/api/farm/pot/skin/buy", `{"skin":"wood"}`); code != http.StatusOK || out["farm"] == nil || out["user"] == nil {
		t.Fatalf("mua kiểu: %d %v", code, out)
	}
	code, out := post("/api/farm/pot/skin/set", `{"floor":1,"slot":0,"skin":"wood"}`)
	if code != http.StatusOK {
		t.Fatalf("lắp kiểu: %d %v", code, out)
	}
	var farm struct {
		Cloud struct {
			Skins []string `json:"skins"`
			Pots  []struct {
				Skin string `json:"skin"`
			} `json:"pots"`
		} `json:"cloud"`
	}
	json.Unmarshal(out["farm"], &farm)
	if len(farm.Cloud.Pots) != 1 || farm.Cloud.Pots[0].Skin != "wood" || !contains(farm.Cloud.Skins, "wood") {
		t.Fatalf("JSON: %s", out["farm"])
	}
	if code, _ := post("/api/farm/pot/skin/set", `{"floor":1,"skin":"wood"}`); code != http.StatusBadRequest {
		t.Fatalf("thiếu slot: %d", code)
	}
	if code, _ := post("/api/farm/pot/skin/buy", `{"skin":"nope"}`); code != http.StatusNotFound {
		t.Fatalf("kiểu lạ: %d", code)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
