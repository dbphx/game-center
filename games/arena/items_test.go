package arena

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dbphx/game-center/internal/wallet"
)

// Danh mục nón / áo: 6 món mỗi loại, món đầu miễn phí, giá và chỉ số tăng dần, món đắt nhất ~0.8× tủ lạnh.
func TestItemCatalogs(t *testing.T) {
	sum := func(a Arm) int { return a.Atk + a.Def + a.Luck + a.Sta }
	tulanh, _ := findArm("tulanh")
	prices := []int64{0, 20000, 40000, 80000, 150000, 300000}
	for slot, list := range map[string][]Arm{SlotHat: Hats, SlotArmor: Armors} {
		if len(list) != 6 || list[0].ID != defaultItem(slot) {
			t.Fatalf("%s: %d món, mặc định %s", slot, len(list), list[0].ID)
		}
		for i, it := range list {
			if it.Price != prices[i] {
				t.Fatalf("%s giá %d", it.ID, it.Price)
			}
			if i > 0 && sum(it) <= sum(list[i-1]) {
				t.Fatalf("%s phải mạnh hơn %s", it.ID, list[i-1].ID)
			}
			if s, ok := slotOfItem(it.ID); !ok || s != slot {
				t.Fatalf("id %s phải thuộc ô %s", it.ID, slot)
			}
		}
		if top := float64(sum(list[5])) / float64(sum(tulanh)); top < 0.75 || top > 0.85 {
			t.Fatalf("%s đắt nhất = %.2f× tủ lạnh", slot, top)
		}
	}
}

// Nón / áo: mua, đội / mặc (chỉ đổi món cùng ô), cường hoá món đang dùng, chuyển cấp trong cùng ô, ô khảm riêng từng món.
func TestHatArmorItems(t *testing.T) {
	e := newArmory(t, 1000000)
	if _, err := e.a.BuyItem(ctx, e.uid, SlotHat, "giaprong"); !errors.Is(err, ErrBadArm) {
		t.Fatalf("áo không mua ở ô nón: %v", err)
	}
	if _, err := e.a.BuyItem(ctx, e.uid, SlotHat, HatDefault); !errors.Is(err, ErrOwned) {
		t.Fatalf("nón lá có sẵn: %v", err)
	}
	if _, err := e.a.EquipItem(ctx, e.uid, SlotHat, "caoboi"); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("chưa mua: %v", err)
	}
	inv, err := e.a.BuyItem(ctx, e.uid, SlotHat, "caoboi")
	if err != nil || inv.Gear.HatID != HatDefault || e.balance() != 920000 {
		t.Fatalf("mua nón cao bồi: %v %+v %d", err, inv.Gear, e.balance())
	}
	if _, err := e.a.BuyItem(ctx, e.uid, SlotArmor, "giapsat"); err != nil {
		t.Fatal(err)
	}
	e.setLevel(SlotHat, 7) // nón lá +7
	e.setLevel(SlotGun, 2)
	before, _ := e.a.Inventory(ctx, e.uid)
	inv, err = e.a.EquipItem(ctx, e.uid, SlotHat, "caoboi")
	if err != nil || inv.Gear.HatID != "caoboi" || inv.Gear.Hat != 0 || inv.Gear.Arm != ArmCannon || inv.Gear.Gun != 2 ||
		inv.Gear.ArmorID != ArmorDefault {
		t.Fatalf("đội nón cao bồi không đổi vũ khí / áo: %v %+v", err, inv.Gear)
	}
	// Nón lá +7: 0/34/34/170 → nón cao bồi +0: 70/100/80/320.
	if inv.Gear.Power() != before.Gear.Power()+2*((70+100+80+320)-(0+34+34+170)) {
		t.Fatalf("lực chiến đổi nón: %d → %d", before.Gear.Power(), inv.Gear.Power())
	}
	inv, err = e.a.EquipItem(ctx, e.uid, SlotArmor, "giapsat")
	if err != nil || inv.Gear.ArmorID != "giapsat" || inv.Gear.HatID != "caoboi" {
		t.Fatalf("mặc giáp sắt: %v %+v", err, inv.Gear)
	}

	// Cường hoá ô nón = nón đang đội.
	tx, _ := e.w.Begin(ctx)
	addStones(ctx, tx, e.uid, 1, 2)
	tx.Commit()
	e.next = 0
	inv, out, err := e.a.Upgrade(ctx, e.uid, SlotHat, 1, false, false)
	if err != nil || out.Item != "caoboi" || out.Arm != "" || inv.Arms["caoboi"] != 1 || inv.Gear.Hat != 1 || inv.Arms[HatDefault] != 7 {
		t.Fatalf("cường hoá nón cao bồi: %v %+v %+v", err, out, inv.Arms)
	}

	// Chuyển cấp nón lá +7 → nón cao bồi; không chuyển chéo ô.
	if _, _, err := e.a.TransferItem(ctx, e.uid, SlotHat, HatDefault, "giapsat"); !errors.Is(err, ErrBadArm) {
		t.Fatalf("chuyển sang áo: %v", err)
	}
	inv, tr, err := e.a.TransferItem(ctx, e.uid, SlotHat, HatDefault, "caoboi")
	if err != nil || tr != (Transfer{SlotHat, HatDefault, "caoboi", 7}) || inv.Arms["caoboi"] != 7 || inv.Arms[HatDefault] != 0 ||
		inv.Gear.Hat != 7 || inv.Gear.HatID != "caoboi" {
		t.Fatalf("chuyển cấp nón: %v %+v %+v", err, tr, inv.Gear)
	}

	// Ô khảm riêng từng món: khảm nón cao bồi +7 (2 ô), đổi về nón lá thì ngọc không còn hiệu lực.
	def2 := GemKey{GemDef, 2}
	e.a.GrantLoot(ctx, e.uid, []GemKey{def2, def2}, 0, "thưởng")
	if _, err := e.a.SocketSlotGem(ctx, e.uid, SlotHat, "", 1, def2); err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.SocketSlotGem(ctx, e.uid, SlotHat, HatDefault, 0, def2); !errors.Is(err, ErrLocked) {
		t.Fatalf("nón lá +0 khoá ô: %v", err)
	}
	if _, err := e.a.SocketSlotGem(ctx, e.uid, SlotHat, "vuongmien", 0, def2); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("chưa có vương miện: %v", err)
	}
	if _, err := e.a.SocketSlotGem(ctx, e.uid, SlotArmor, "caoboi", 0, def2); !errors.Is(err, ErrBadArm) {
		t.Fatalf("nón không thuộc ô áo: %v", err)
	}
	inv, _ = e.a.Inventory(ctx, e.uid)
	if !inv.Sockets["caoboi"][1].valid() || inv.Gear.Bonus.Def != 40 {
		t.Fatalf("ngọc nón cao bồi: %+v %+v", inv.Sockets, inv.Gear.Bonus)
	}
	r := reply(inv, 0)
	if r.Hats[3].ID != "caoboi" || !r.Hats[3].Equipped || r.Hats[3].Level != 7 || r.Hats[3].Sockets[1].Gem == nil ||
		r.Hats[3].Eff.Def != Hats[3].Eff(7).Def+40 || r.SlotSockets[SlotHat][1].Gem == nil || r.Armors[4].ID != "giapsat" ||
		!r.Armors[4].Equipped || !r.Armors[4].Owned || r.Armors[5].Owned || r.Gear.ArmorID != "giapsat" {
		t.Fatalf("reply hats/armors: %+v / %+v", r.Hats[3], r.Armors[4])
	}
	inv, _ = e.a.EquipItem(ctx, e.uid, SlotHat, HatDefault)
	if inv.Gear.Bonus.Def != 0 || !inv.Sockets["caoboi"][1].valid() {
		t.Fatalf("ngọc ở nón không đội thì không tính: %+v", inv.Gear.Bonus)
	}
	// Chuyển cấp nón cao bồi đi → ô bị khoá, ngọc về kho.
	e.a.EquipItem(ctx, e.uid, SlotHat, "caoboi")
	if _, err := e.a.BuyItem(ctx, e.uid, SlotHat, "vuongmien"); err != nil {
		t.Fatal(err)
	}
	inv, _, err = e.a.TransferItem(ctx, e.uid, SlotHat, "caoboi", "vuongmien")
	if err != nil || inv.Sockets["caoboi"][1].valid() || inv.Gems[def2] != 2 || inv.Arms["vuongmien"] != 7 {
		t.Fatalf("chuyển cấp trả ngọc: %v %+v %+v", err, inv.Sockets, inv.Gems)
	}
}

// Máu / giảm sát thương / lực chiến tính từ tổng chỉ số vũ khí + nón + áo.
func TestGearTotals(t *testing.T) {
	g := Gear{}
	if s := g.Stats(); s != (ArmStats{100, 110, 40, 350}) || g.MaxHP() != 2350 || g.Power() != 11200 || g.ArmorPct() != 0.11 {
		t.Fatalf("đồ mặc định: %+v hp %d power %d", s, g.MaxHP(), g.Power())
	}
	top := Gear{Arm: "tulanh", Gun: 10, HatID: "vuongmien", Hat: 10, ArmorID: "giaprong", Armor: 10}
	// Tủ lạnh +10 1100/640/240/1200, vương miện +10 320/360/260/1600, giáp rồng +10 200/640/100/1600.
	if s := top.Stats(); s != (ArmStats{1620, 1640, 600, 4400}) || top.MaxHP() != 6400 || top.ArmorPct() != MaxArmorPct {
		t.Fatalf("đồ tối đa: %+v hp %d", s, top.MaxHP())
	}
	if n := (Gear{HatID: "x"}).Norm(); n.Arm != ArmCannon || n.HatID != HatDefault || n.ArmorID != ArmorDefault {
		t.Fatalf("Norm: %+v", n)
	}
}

// Database cũ: cấp nón / áo trong arena_gear và ngọc ở ô "hat" / "armor" chuyển sang nón lá / áo thun.
func TestMigrateOldHatArmor(t *testing.T) {
	w, err := wallet.Open(filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := New(ctx, w); err != nil { // tạo bảng như bản trước
		t.Fatal(err)
	}
	u, _, _ := w.Register(ctx, "nguoicu", "matkhau123", wallet.Profile{Name: "Cũ"})
	tx, _ := w.Begin(ctx)
	tx.ExecContext(ctx, `INSERT INTO arena_gear (user_id, slot, level) VALUES (?, 'hat', 7), (?, 'armor', 3)`, u.ID, u.ID)
	tx.ExecContext(ctx, `INSERT INTO arena_sockets (user_id, arm, idx, kind, tier) VALUES (?, 'hat', 1, 'def', 2), (?, 'armor', 0, 'sta', 1)`, u.ID, u.ID)
	tx.Commit()
	a, err := New(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := a.Inventory(ctx, u.ID)
	if err != nil || inv.Gear.HatID != HatDefault || inv.Gear.Hat != 7 || inv.Gear.ArmorID != ArmorDefault || inv.Gear.Armor != 3 ||
		inv.Sockets[HatDefault][1] != (GemKey{GemDef, 2}) || inv.Sockets[ArmorDefault][0] != (GemKey{GemSta, 1}) ||
		inv.Gear.Bonus != (ArmStats{Def: 40, Sta: 50}) {
		t.Fatalf("migrate nón/áo: %v %+v %+v", err, inv.Gear, inv.Sockets)
	}
	// Khởi động lại không ghi đè cấp đã đổi.
	tx, _ = w.Begin(ctx)
	setArm(ctx, tx, u.ID, HatDefault, 2, true)
	tx.Commit()
	a, _ = New(ctx, w)
	if g, _ := a.Gear(ctx, u.ID); g.Hat != 2 {
		t.Fatalf("migrate chạy lại: %+v", g)
	}
}

// API /api/arena/items/*: mua, dùng, chuyển cấp theo ô; khảm theo item.
func TestItemsAPI(t *testing.T) {
	e := newArmory(t, 200000)
	srv := httptest.NewServer(e.a.Handler())
	defer srv.Close()
	post := func(path string, body any) (int, GearReply) {
		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(body)
		req, _ := http.NewRequest("POST", srv.URL+path, &buf)
		req.Header.Set("Authorization", "Bearer "+e.token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var g GearReply
		json.NewDecoder(res.Body).Decode(&g)
		return res.StatusCode, g
	}
	if code, g := post("/api/arena/items/buy", map[string]any{"slot": "armor", "id": "aophao"}); code != 200 || !g.Armors[3].Owned || g.Balance != 120000 {
		t.Fatalf("mua áo phao: %d %+v", code, g.Armors)
	}
	if code, _ := post("/api/arena/items/buy", map[string]any{"slot": "armor", "id": "aophao"}); code != 409 {
		t.Fatalf("mua lại: %d", code)
	}
	if code, _ := post("/api/arena/items/buy", map[string]any{"slot": "hat", "id": "aophao"}); code != 400 {
		t.Fatalf("sai ô: %d", code)
	}
	if code, _ := post("/api/arena/items/buy", map[string]any{"slot": "hat", "id": "vuongmien"}); code != 402 {
		t.Fatalf("thiếu xu: %d", code)
	}
	if code, g := post("/api/arena/items/buy", map[string]any{"id": "dep"}); code != 200 || !g.Arms[1].Owned {
		t.Fatalf("không có slot = vũ khí: %d", code)
	}
	code, g := post("/api/arena/items/equip", map[string]any{"slot": "armor", "id": "aophao"})
	if code != 200 || g.Gear.ArmorID != "aophao" || !g.Armors[3].Equipped || g.Armors[0].Equipped {
		t.Fatalf("mặc áo phao: %d %+v", code, g.Gear)
	}
	if code, _ := post("/api/arena/items/equip", map[string]any{"slot": "armor", "id": "giaprong"}); code != 409 {
		t.Fatalf("chưa có: %d", code)
	}
	e.setLevel(SlotArmor, 4)
	if code, _ := post("/api/arena/items/transfer", map[string]any{"slot": "armor", "from": "aothun", "to": "aophao"}); code != 409 {
		t.Fatalf("áo thun +0: %d", code)
	}
	code, g = post("/api/arena/items/transfer", map[string]any{"slot": "armor", "from": "aophao", "to": "aothun"})
	if code != 200 || g.Transfer == nil || *g.Transfer != (Transfer{SlotArmor, "aophao", "aothun", 4}) || g.Armors[0].Level != 4 ||
		g.Armors[3].Level != 0 || g.Gear.Armor != 0 || g.Balance != 120000-15000-12000 {
		t.Fatalf("chuyển cấp áo: %d %+v %d", code, g.Transfer, g.Balance)
	}
	// Khảm theo item (không phải món đang dùng).
	e.a.GrantLoot(ctx, e.uid, []GemKey{{GemSta, 3}}, 0, "thưởng")
	code, g = post("/api/arena/gems/socket", map[string]any{"slot": "armor", "item": "aothun", "idx": 0, "kind": "sta", "tier": 3})
	if code != 200 || g.Armors[0].Sockets[0].Gem == nil || g.Gear.Bonus.Sta != 0 || g.Armors[0].Eff.Sta != Armors[0].Eff(4).Sta+180 {
		t.Fatalf("khảm áo thun (không mặc): %d %+v", code, g.Armors[0])
	}
	code, g = post("/api/arena/gems/unsocket", map[string]any{"slot": "armor", "item": "aothun", "idx": 0})
	if code != 200 || g.Armors[0].Sockets[0].Gem != nil || len(g.Gems) != 1 {
		t.Fatalf("tháo ngọc áo thun: %d", code)
	}
}
