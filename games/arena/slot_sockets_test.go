package arena

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Nón và áo giáp cũng có 3 ô khảm (+3/+7/+10); ngọc cộng vào chỉ số chiến đấu cùng ngọc vũ khí.
func TestHatArmorSockets(t *testing.T) {
	e := newArmory(t, 100000)
	def2, sta1, atk1 := GemKey{GemDef, 2}, GemKey{GemSta, 1}, GemKey{GemAtk, 1}
	e.a.GrantLoot(ctx, e.uid, []GemKey{def2, def2, sta1, atk1}, 0, "thưởng")

	if _, err := e.a.SocketSlotGem(ctx, e.uid, SlotHat, "", 0, def2); !errors.Is(err, ErrLocked) {
		t.Fatalf("nón +0 chưa có ô: %v", err)
	}
	if _, err := e.a.SocketSlotGem(ctx, e.uid, "boots", "", 0, def2); !errors.Is(err, ErrBadSlot) {
		t.Fatalf("ô trang bị lạ: %v", err)
	}
	e.setLevel(SlotHat, 7)
	e.setLevel(SlotArmor, 3)
	e.setLevel(SlotGun, 3)
	before, _ := e.a.Inventory(ctx, e.uid)
	if _, err := e.a.SocketSlotGem(ctx, e.uid, SlotArmor, "", 1, def2); !errors.Is(err, ErrLocked) {
		t.Fatalf("áo +3 chỉ mở ô 0: %v", err)
	}
	if _, err := e.a.SocketSlotGem(ctx, e.uid, SlotHat, "", 0, def2); err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.SocketSlotGem(ctx, e.uid, SlotHat, "", 1, sta1); err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.SocketSlotGem(ctx, e.uid, SlotArmor, "", 0, def2); err != nil {
		t.Fatal(err)
	}
	inv, err := e.a.SocketGem(ctx, e.uid, ArmCannon, 0, atk1) // vũ khí: tương thích cũ
	if err != nil {
		t.Fatal(err)
	}
	want := ArmStats{Atk: 30, Def: 40 + 40, Sta: 50}
	if inv.Gear.Bonus != want || len(inv.Gems) != 0 {
		t.Fatalf("bonus gộp vũ khí + nón + áo: %+v %+v", inv.Gear.Bonus, inv.Gems)
	}
	if inv.Gear.Power() != before.Gear.Power()+2*(30+80+50) || inv.Gear.MaxHP() != before.Gear.MaxHP()+50 {
		t.Fatalf("ngọc nón/áo phải tăng lực chiến: %d → %d", before.Gear.Power(), inv.Gear.Power())
	}
	r := reply(inv, 0)
	if s := r.SlotSockets[SlotHat]; len(s) != 3 || !s[1].Open || s[2].Open || s[0].Gem == nil || s[0].Gem.Kind != GemDef || s[2].UnlockAt != 10 {
		t.Fatalf("slotSockets nón: %+v", s)
	}
	if s := r.SlotSockets[SlotArmor]; len(s) != 3 || !s[0].Open || s[1].Open || s[0].Gem == nil {
		t.Fatalf("slotSockets áo: %+v", s)
	}
	if r.Stats.Def != inv.Gear.Stats().Def || r.Arms[0].Eff.Def != Arms[0].Eff(3).Def {
		t.Fatalf("stats gồm ngọc nón/áo, eff vũ khí thì không: %+v %+v", r.Stats, r.Arms[0].Eff)
	}

	// Nón cường hoá thất bại +7 → +6: ô 1 khoá, Ngọc Thể lực I về kho; ô 0 giữ.
	tx, _ := e.w.Begin(ctx)
	addStones(ctx, tx, e.uid, 1, 1)
	tx.Commit()
	e.next = 99
	inv, out, err := e.a.Upgrade(ctx, e.uid, SlotHat, 1, false, false)
	if err != nil || !out.Dropped || out.Level != 6 || inv.Gems[sta1] != 1 || inv.Sockets[HatDefault][1].valid() ||
		!inv.Sockets[HatDefault][0].valid() || inv.Gear.Bonus != (ArmStats{Atk: 30, Def: 80}) {
		t.Fatalf("nón tụt cấp khoá ô: %v %+v %+v %+v", err, out, inv.Gems, inv.Gear.Bonus)
	}

	// Tháo ngọc áo: về kho; ô trống 409.
	inv, err = e.a.UnsocketSlotGem(ctx, e.uid, SlotArmor, "", 0)
	if err != nil || inv.Gems[def2] != 1 || inv.Gear.Bonus.Def != 40 {
		t.Fatalf("tháo ngọc áo: %v %+v", err, inv.Gems)
	}
	if _, err := e.a.UnsocketSlotGem(ctx, e.uid, SlotArmor, "", 0); !errors.Is(err, ErrEmptySlot) {
		t.Fatalf("ô áo trống: %v", err)
	}

	// HTTP: có slot → nón/áo; không có slot → vũ khí như cũ.
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
	code, g := post("/api/arena/gems/socket", map[string]any{"slot": "armor", "idx": 0, "kind": "def", "tier": 2})
	if code != 200 || g.SlotSockets[SlotArmor][0].Gem == nil || g.Gear.Bonus.Def != 80 {
		t.Fatalf("khảm áo qua API: %d %+v", code, g.SlotSockets)
	}
	code, g = post("/api/arena/gems/unsocket", map[string]any{"arm": "cannon", "idx": 0})
	if code != 200 || g.Arms[0].Sockets[0].Gem != nil || g.Gear.Bonus.Atk != 0 {
		t.Fatalf("tháo vũ khí không có slot: %d %+v", code, g.Arms[0].Sockets)
	}
	if code, _ := post("/api/arena/gems/socket", map[string]any{"slot": "hat", "idx": 2, "kind": "sta", "tier": 1}); code != 409 {
		t.Fatalf("ô nón khoá: %d", code)
	}
	if code, _ := post("/api/arena/gems/socket", map[string]any{"slot": "boots", "idx": 0, "kind": "sta", "tier": 1}); code != 400 {
		t.Fatalf("slot lạ: %d", code)
	}
}
