package arena

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dbphx/game-center/internal/wallet"
)

func TestProfile(t *testing.T) {
	e := newArmory(t, 100000)
	// Xạ thủ mua + đội Mũ bảo hiểm +7, khảm 1 ngọc Thể lực III vào nón.
	if _, err := e.a.BuyItem(ctx, e.uid, SlotHat, "mubh"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.EquipItem(ctx, e.uid, SlotHat, "mubh"); err != nil {
		t.Fatal(err)
	}
	e.setLevel(SlotHat, 7)
	if err := e.a.GrantLoot(ctx, e.uid, []GemKey{{"sta", 3}}, 0, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.SocketSlotGem(ctx, e.uid, SlotHat, "", 0, GemKey{"sta", 3}); err != nil {
		t.Fatal(err)
	}
	// Người xem khác.
	_, token, err := e.w.Register(ctx, "nguoixem", "matkhau123", wallet.Profile{Name: "Người xem"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(e.a.Handler())
	defer srv.Close()
	get := func(path, tok string) (int, string) {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	if code, _ := get("/api/arena/players/xathu", ""); code != 401 {
		t.Fatalf("chưa đăng nhập: %d", code)
	}
	if code, body := get("/api/arena/players/khongco", token); code != 404 || !strings.Contains(body, "error") {
		t.Fatalf("không tồn tại: %d %s", code, body)
	}
	code, body := get("/api/arena/players/XaThu", token)
	if code != 200 {
		t.Fatalf("hồ sơ: %d %s", code, body)
	}
	for _, secret := range []string{"balance", "stones", "charms", "gems\""} {
		if strings.Contains(body, secret) {
			t.Fatalf("hồ sơ không được lộ %q: %s", secret, body)
		}
	}
	var p Profile
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	inv, _ := e.a.Inventory(ctx, e.uid)
	g := inv.Gear
	if p.Username != "xathu" || p.Name != "Xạ thủ" || p.Power != g.Power() || p.Stats.MaxHP != g.MaxHP() ||
		p.Stats.Sta != g.Stats().Sta || p.Stats.CritPct != g.CritPct() {
		t.Fatalf("hồ sơ chỉ số: %+v", p)
	}
	h := p.Hat
	if h.ID != "mubh" || h.Level != 7 || h.Form != 2 || h.Sta != 150 || h.Eff.Sta != 255+180 || len(h.Sockets) != 3 ||
		h.Sockets[0].Gem == nil || !h.Sockets[1].Open || h.Sockets[2].Open {
		t.Fatalf("nón trong hồ sơ: %+v", h)
	}
	if p.Weapon.ID != ArmCannon || p.Weapon.Form != 1 || p.Armor.ID != ArmorDefault {
		t.Fatalf("vũ khí / áo mặc định: %+v %+v", p.Weapon, p.Armor)
	}
	if Form(6) != 1 || Form(7) != 2 || Form(9) != 2 || Form(10) != 3 {
		t.Fatal("dạng hình theo cấp")
	}
}

// Trong state, người thật có username để mở hồ sơ; máy để rỗng.
func TestStateUsername(t *testing.T) {
	h := NewHub(newBank(), fast, nil)
	ca := &fakeConn{}
	u := an
	u.Username = "an"
	sa, err := h.Create(u, ca, ptr(0), "meadow", "", "")
	if err != nil {
		t.Fatal(err)
	}
	do(t, sa, Message{T: "addBot", Level: BotEasy})
	st := ca.last()
	if len(st.Players) != 2 || st.Players[0].Username != "an" || st.Players[1].Username != "" {
		t.Fatalf("username trong state: %+v", st.Players)
	}
}
