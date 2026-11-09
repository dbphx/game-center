package farm

import (
	"net/http"
	"sync"
	"testing"
)

// grantVip tặng n chậu VIP (như ghép từ mảnh nhiệm vụ).
func (e *env) grantVip(skin string, n int) {
	e.t.Helper()
	for range n {
		tx, err := e.w.Begin(ctx)
		if err != nil {
			e.t.Fatal(err)
		}
		if err := GrantVipPot(ctx, tx, e.uid, skin); err != nil {
			e.t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			e.t.Fatal(err)
		}
	}
}

func (e *env) view() State {
	e.t.Helper()
	return e.ok(e.f.State(ctx, e.uid))
}

func TestVipPotCatalog(t *testing.T) {
	want := map[string]int{"vip_gift": 15, "vip_snowman": 30, "vip_pine": 50}
	got := map[string]int{}
	for _, s := range PotSkins {
		if s.VIP {
			if s.Price != 0 || s.Name == "" {
				t.Fatalf("chậu VIP không có giá xu: %+v", s)
			}
			got[s.ID] = s.SpeedPct
		} else if s.SpeedPct != 0 {
			t.Fatalf("chậu thường không rút ngắn thời gian: %+v", s)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("chậu VIP: %v", got)
	}
	for id, pct := range want {
		if got[id] != pct {
			t.Fatalf("%s rút ngắn %d%%, muốn %d%%", id, got[id], pct)
		}
	}
	if len(VipPots()) != 3 || VipPots()[2].ID != "vip_pine" {
		t.Fatalf("VipPots theo bậc: %+v", VipPots())
	}
}

func TestVipPotNotBuyableNotFree(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.openFloors(1)
	s := e.ok(e.f.BuyPot(ctx, e.uid, 1))
	if contains(s.Cloud.Skins, "vip_gift") {
		t.Fatal("chậu VIP không có sẵn")
	}
	before := e.user().Balance
	_, err := e.f.BuyPotSkin(ctx, e.uid, "vip_gift")
	wantStatus(t, err, http.StatusConflict)
	_, err = e.f.SetPotSkin(ctx, e.uid, 1, 0, "vip_gift")
	wantStatus(t, err, http.StatusConflict)
	if e.user().Balance != before {
		t.Fatal("không được trừ xu cho chậu VIP")
	}
}

// Lắp: mỗi chậu VIP chiếm một chỗ; lắp quá số có → 409; tháo / đổi kiểu khác trả lại kho; lắp lại cùng chỗ không tốn thêm.
func TestVipPotInstallCount(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.openFloors(2)
	e.ok(e.f.BuyPot(ctx, e.uid, 1))
	e.ok(e.f.BuyPot(ctx, e.uid, 1))
	e.ok(e.f.BuyPot(ctx, e.uid, 2))
	e.grantVip("vip_pine", 2)
	s := e.view()
	if v := s.Cloud.Vip["vip_pine"]; v.Owned != 2 || v.Used != 0 || !contains(s.Cloud.Skins, "vip_pine") {
		t.Fatalf("kho chậu VIP: %+v %v", s.Cloud.Vip, s.Cloud.Skins)
	}
	e.ok(e.f.SetPotSkin(ctx, e.uid, 1, 0, "vip_pine"))
	e.ok(e.f.SetPotSkin(ctx, e.uid, 1, 0, "vip_pine")) // lắp lại cùng chỗ
	s = e.ok(e.f.SetPotSkin(ctx, e.uid, 2, 0, "vip_pine"))
	if v := s.Cloud.Vip["vip_pine"]; v.Used != 2 {
		t.Fatalf("đang lắp: %+v", v)
	}
	_, err := e.f.SetPotSkin(ctx, e.uid, 1, 1, "vip_pine")
	wantStatus(t, err, http.StatusConflict)
	// Tháo ở tầng 2 → lắp được ở chỗ khác.
	e.ok(e.f.SetPotSkin(ctx, e.uid, 2, 0, "white"))
	s = e.ok(e.f.SetPotSkin(ctx, e.uid, 1, 1, "vip_pine"))
	if v := s.Cloud.Vip["vip_pine"]; v.Owned != 2 || v.Used != 2 {
		t.Fatalf("sau khi chuyển: %+v", v)
	}
	if p, _ := s.Cloud.pot(2, 0); p.Skin != "white" {
		t.Fatalf("tầng 2: %+v", p)
	}
	_, err = e.f.SetPotSkin(ctx, e.uid, 1, 1, "vip_gift")
	wantStatus(t, err, http.StatusConflict) // chưa có loại này
}

// Thời gian chín tính lúc gieo theo chậu; đổi chậu khi cây đang lớn không đổi cây đó.
func TestVipPotShortensGrowth(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.openFloors(1)
	for range 4 {
		e.ok(e.f.BuyPot(ctx, e.uid, 1))
	}
	e.grantVip("vip_gift", 1)
	e.grantVip("vip_snowman", 1)
	e.grantVip("vip_pine", 1)
	e.ok(e.f.SetPotSkin(ctx, e.uid, 1, 1, "vip_gift"))
	e.ok(e.f.SetPotSkin(ctx, e.uid, 1, 2, "vip_snowman"))
	e.ok(e.f.SetPotSkin(ctx, e.uid, 1, 3, "vip_pine"))
	var s State
	for slot := range 4 {
		s = e.ok(e.f.PlantPot(ctx, e.uid, 1, slot))
	}
	grow := CloudFloors[0].Crop.GrowSeconds
	for slot, pct := range []int64{0, 15, 30, 50} {
		p, _ := s.Cloud.pot(1, slot)
		if got, want := p.ReadyAt-p.PlantedAt, grow*(100-pct)/100; got != want {
			t.Fatalf("chậu %d: chín sau %d giây, muốn %d", slot, got, want)
		}
	}
	// Tháo chậu Thông Vàng khi cây đang lớn: cây giữ giờ chín cũ.
	p3, _ := s.Cloud.pot(1, 3)
	s = e.ok(e.f.SetPotSkin(ctx, e.uid, 1, 3, "default"))
	if p, _ := s.Cloud.pot(1, 3); p.ReadyAt != p3.ReadyAt {
		t.Fatalf("đổi chậu làm đổi giờ chín: %d → %d", p3.ReadyAt, p.ReadyAt)
	}
}

// Nhiều lần lắp cùng lúc không vượt số chậu đang có.
func TestVipPotConcurrentInstall(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.openFloors(1)
	for range 6 {
		e.ok(e.f.BuyPot(ctx, e.uid, 1))
	}
	e.grantVip("vip_snowman", 2)
	var wg sync.WaitGroup
	for slot := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.f.SetPotSkin(ctx, e.uid, 1, slot, "vip_snowman")
		}()
	}
	wg.Wait()
	s := e.view()
	used := 0
	for _, p := range s.Cloud.Pots {
		if p.Skin == "vip_snowman" {
			used++
		}
	}
	if used != 2 || s.Cloud.Vip["vip_snowman"].Used != 2 {
		t.Fatalf("lắp vượt số: %d chỗ, %+v", used, s.Cloud.Vip)
	}
}

func TestGrantVipPotUnknown(t *testing.T) {
	e := newEnv(t, 0, 0)
	tx, err := e.w.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := GrantVipPot(ctx, tx, e.uid, "gold"); err == nil {
		t.Fatal("chỉ chậu VIP mới tặng được")
	}
}
