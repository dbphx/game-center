package arena

import (
	"errors"
	"math"
	"path/filepath"
	"testing"

	"github.com/dbphx/game-center/internal/wallet"
)

func TestArmsBuyEquipTransfer(t *testing.T) {
	e := newArmory(t, 1000)
	if _, err := e.a.BuyArm(ctx, e.uid, "giay"); !errors.Is(err, ErrBadArm) {
		t.Fatalf("vũ khí lạ: %v", err)
	}
	if _, err := e.a.BuyArm(ctx, e.uid, ArmCannon); !errors.Is(err, ErrOwned) {
		t.Fatalf("đại bác ai cũng có: %v", err)
	}
	if _, err := e.a.BuyArm(ctx, e.uid, "dep"); !errors.Is(err, wallet.ErrNotEnoughCoins) {
		t.Fatalf("thiếu xu: %v", err)
	}
	if _, err := e.a.EquipArm(ctx, e.uid, "dep"); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("chưa có thì không trang bị: %v", err)
	}
	e.setBalance(30000)
	inv, err := e.a.BuyArm(ctx, e.uid, "dep")
	if err != nil || e.balance() != 28500 || inv.Gear.ArmID() != ArmCannon {
		t.Fatalf("mua dép lào: %v ví %d %+v", err, e.balance(), inv.Gear)
	}
	if _, err := e.a.BuyArm(ctx, e.uid, "dep"); !errors.Is(err, ErrOwned) {
		t.Fatalf("mua lại: %v", err)
	}
	e.a.BuyArm(ctx, e.uid, "tulanh")
	// Đại bác +5, cường hoá xong chuyển sang tủ lạnh.
	e.setLevel(SlotGun, 5)
	if _, _, err := e.a.TransferArm(ctx, e.uid, ArmCannon, ArmCannon); !errors.Is(err, ErrSameArm) {
		t.Fatalf("cùng vũ khí: %v", err)
	}
	if _, _, err := e.a.TransferArm(ctx, e.uid, ArmCannon, "tv"); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("chưa có TV: %v", err)
	}
	if _, _, err := e.a.TransferArm(ctx, e.uid, "dep", ArmCannon); !errors.Is(err, ErrNoLevel) {
		t.Fatalf("nguồn +0: %v", err)
	}
	e.setBalance(1000)
	if _, _, err := e.a.TransferArm(ctx, e.uid, ArmCannon, "tulanh"); !errors.Is(err, wallet.ErrNotEnoughCoins) {
		t.Fatalf("phí 1500: %v", err)
	}
	e.setBalance(2000)
	inv, tr, err := e.a.TransferArm(ctx, e.uid, ArmCannon, "tulanh")
	if err != nil || tr != (Transfer{ArmCannon, "tulanh", 5}) || inv.Arms["tulanh"] != 5 || inv.Arms[ArmCannon] != 0 ||
		inv.Gear.Gun != 0 || e.balance() != 500 {
		t.Fatalf("chuyển cấp: %v %+v %+v ví %d", err, tr, inv.Arms, e.balance())
	}
	if _, _, err := e.a.TransferArm(ctx, e.uid, "dep", "tulanh"); !errors.Is(err, ErrNoLevel) {
		t.Fatalf("nguồn +0: %v", err)
	}
	// Trang bị tủ lạnh: cấp +5, chỉ số ×1.5.
	inv, err = e.a.EquipArm(ctx, e.uid, "tulanh")
	if err != nil || inv.Gear.Arm != "tulanh" || inv.Gear.Gun != 5 {
		t.Fatalf("trang bị: %v %+v", err, inv.Gear)
	}
	if s := inv.Gear.Stats(); s != (ArmStats{83, 48, 18, 90}) {
		t.Fatalf("tủ lạnh +5: %+v", s)
	}
	if inv.Gear.MaxHP() != 290 || math.Abs(inv.Gear.DmgMul()-1.83) > 1e-9 || inv.Gear.CritPct() != 18 {
		t.Fatalf("hiệu ứng: %d %v", inv.Gear.MaxHP(), inv.Gear.DmgMul())
	}
	// Cường hoá ô súng = vũ khí đang trang bị.
	e.setBalance(1000)
	e.a.BuyStones(ctx, e.uid, 5, 0)
	tx, _ := e.w.Begin(ctx)
	addStones(ctx, tx, e.uid, 5, 1)
	tx.Commit()
	e.next = 0
	inv, out, err := e.a.Upgrade(ctx, e.uid, SlotGun, 5, false, false)
	if err != nil || out.Arm != "tulanh" || inv.Arms["tulanh"] != 6 || inv.Arms[ArmCannon] != 0 {
		t.Fatalf("cường hoá tủ lạnh: %v %+v %+v", err, out, inv.Arms)
	}
	// Không chuyển ngược khi nguồn thấp hơn đích.
	e.setLevel(SlotGun, 6)
	e.a.EquipArm(ctx, e.uid, "dep")
	e.setLevel(SlotGun, 2)
	if _, _, err := e.a.TransferArm(ctx, e.uid, "dep", "tulanh"); !errors.Is(err, ErrLowLevel) {
		t.Fatalf("nguồn thấp hơn: %v", err)
	}
	r := reply(inv, 0)
	if len(r.Arms) != 6 || r.TransferFeePerLevel != 300 || !r.Arms[0].Owned || r.Arms[3].Owned {
		t.Fatalf("reply arms: %+v", r.Arms)
	}
}

func TestMigrateOldGun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	w, err := wallet.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	// Database cũ: chỉ có arena_gear với ô súng +4.
	w.ExecSchema(ctx, `CREATE TABLE IF NOT EXISTS arena_gear (user_id INTEGER NOT NULL REFERENCES users(id),
		slot TEXT NOT NULL, level INTEGER NOT NULL, PRIMARY KEY (user_id, slot));`)
	u, _, err := w.Register(ctx, "nguoicu", "matkhau123", wallet.Profile{Name: "Cũ"})
	if err != nil {
		t.Fatal(err)
	}
	tx, _ := w.Begin(ctx)
	tx.ExecContext(ctx, `INSERT INTO arena_gear (user_id, slot, level) VALUES (?, 'gun', 4), (?, 'hat', 2)`, u.ID, u.ID)
	tx.Commit()
	a, err := New(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	g, err := a.Gear(ctx, u.ID)
	if err != nil || g != (Gear{Arm: ArmCannon, Gun: 4, Hat: 2}) {
		t.Fatalf("chuyển súng cũ: %+v %v", g, err)
	}
	// Chạy lại (khởi động lần sau) không ghi đè cấp đã đổi.
	tx, _ = w.Begin(ctx)
	setArm(ctx, tx, u.ID, ArmCannon, 1, true)
	tx.Commit()
	a, _ = New(ctx, w)
	if g, _ := a.Gear(ctx, u.ID); g.Gun != 1 {
		t.Fatalf("migrate chạy lại: %+v", g)
	}
}

func TestGemsSocketLock(t *testing.T) {
	e := newArmory(t, 100000)
	atk2 := GemKey{GemAtk, 2}
	sta3 := GemKey{GemSta, 3}
	if atk2.Name() != "Ngọc Tấn công II" || atk2.Value() != 6 || sta3.Value() != 18 {
		t.Fatalf("ngọc: %s %d", atk2.Name(), atk2.Value())
	}
	if OpenSockets(2) != 0 || OpenSockets(3) != 1 || OpenSockets(7) != 2 || OpenSockets(10) != 3 {
		t.Fatal("ô mở ở +3, +7, +10")
	}
	if err := e.a.GrantLoot(ctx, e.uid, []GemKey{atk2, atk2, sta3}, 100, "thưởng"); err != nil || e.balance() != 100100 {
		t.Fatalf("nhận ngọc: %v", err)
	}
	if _, err := e.a.SocketGem(ctx, e.uid, ArmCannon, 0, atk2); !errors.Is(err, ErrLocked) {
		t.Fatalf("+0 chưa có ô: %v", err)
	}
	e.setLevel(SlotGun, 7)
	before, _ := e.a.Inventory(ctx, e.uid)
	if _, err := e.a.SocketGem(ctx, e.uid, ArmCannon, 2, atk2); !errors.Is(err, ErrLocked) {
		t.Fatalf("ô 3 mở ở +10: %v", err)
	}
	if _, err := e.a.SocketGem(ctx, e.uid, ArmCannon, 0, GemKey{GemLuck, 1}); !errors.Is(err, ErrNoGem) {
		t.Fatalf("không có ngọc: %v", err)
	}
	if _, err := e.a.SocketGem(ctx, e.uid, ArmCannon, 0, GemKey{"x", 1}); !errors.Is(err, ErrBadGem) {
		t.Fatalf("ngọc lạ: %v", err)
	}
	if _, err := e.a.SocketGem(ctx, e.uid, "dep", 0, atk2); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("chưa có dép: %v", err)
	}
	e.a.SocketGem(ctx, e.uid, ArmCannon, 0, atk2)
	inv, err := e.a.SocketGem(ctx, e.uid, ArmCannon, 1, sta3)
	if err != nil || inv.Gems[atk2] != 1 || inv.Gems[sta3] != 0 || inv.Gear.Bonus != (ArmStats{Atk: 6, Sta: 18}) {
		t.Fatalf("khảm: %v %+v %+v", err, inv.Gems, inv.Gear.Bonus)
	}
	if inv.Gear.Power() != before.Gear.Power()+2*(6+18) || inv.Gear.MaxHP() != before.Gear.MaxHP()+18 {
		t.Fatalf("khảm ngọc phải tăng lực chiến: %d → %d", before.Gear.Power(), inv.Gear.Power())
	}
	r := reply(inv, 0)
	if s := r.Arms[0].Sockets; len(s) != 3 || !s[1].Open || s[2].Open || s[1].Gem == nil || s[1].Gem.Kind != GemSta || s[2].UnlockAt != 10 {
		t.Fatalf("ô khảm reply: %+v", s)
	}
	if r.Arms[0].Eff.Atk != 17+6 || len(r.Gems) != 1 || r.Gems[0].Qty != 1 {
		t.Fatalf("eff/gems reply: %+v %+v", r.Arms[0].Eff, r.Gems)
	}
	// Khảm đè: ngọc cũ về kho.
	inv, _ = e.a.SocketGem(ctx, e.uid, ArmCannon, 0, atk2)
	if inv.Gems[atk2] != 1 {
		t.Fatalf("ngọc cũ về kho: %+v", inv.Gems)
	}
	// Cường hoá thất bại +7 → +6: ô 2 khoá, Ngọc Thể lực III về kho.
	tx, _ := e.w.Begin(ctx)
	addStones(ctx, tx, e.uid, 1, 1)
	tx.Commit()
	e.next = 99
	inv, out, err := e.a.Upgrade(ctx, e.uid, SlotGun, 1, false, false)
	if err != nil || !out.Dropped || inv.Gems[sta3] != 1 || inv.Sockets[ArmCannon][1].valid() || !inv.Sockets[ArmCannon][0].valid() ||
		inv.Gear.Bonus != (ArmStats{Atk: 6}) {
		t.Fatalf("tụt cấp khoá ô: %v %+v %+v %+v", err, out, inv.Gems, inv.Sockets)
	}
	// Tháo ngọc: về kho; ô trống thì 409.
	inv, err = e.a.UnsocketGem(ctx, e.uid, ArmCannon, 0)
	if err != nil || inv.Gems[atk2] != 2 || inv.Gear.Bonus != (ArmStats{}) {
		t.Fatalf("tháo: %v %+v", err, inv.Gems)
	}
	if _, err := e.a.UnsocketGem(ctx, e.uid, ArmCannon, 0); !errors.Is(err, ErrEmptySlot) {
		t.Fatalf("ô trống: %v", err)
	}
	// Chuyển cấp đi: vũ khí nguồn về +0, ngọc về kho.
	e.a.SocketGem(ctx, e.uid, ArmCannon, 0, atk2)
	e.a.BuyArm(ctx, e.uid, "dep")
	inv, _, err = e.a.TransferArm(ctx, e.uid, ArmCannon, "dep")
	if err != nil || inv.Gems[atk2] != 2 || inv.Sockets[ArmCannon][0].valid() || inv.Arms["dep"] != 6 {
		t.Fatalf("chuyển cấp trả ngọc: %v %+v %+v", err, inv.Gems, inv.Sockets)
	}
}

func TestCritAndUnlimitedSkills(t *testing.T) {
	for _, w := range Weapons {
		if w.Uses != -1 {
			t.Fatalf("%s phải dùng không giới hạn", w.ID)
		}
	}
	if len(startItems()) != 0 {
		t.Fatal("không còn giới hạn lượt dùng")
	}
	g := flat(700)
	a := &Fighter{ID: "a", X: 200, Y: 700, HP: 200, MaxHP: 200, Alive: true}
	l := Simulate(g, 200, 700, 45, 50, 0, "a", nil)
	b := &Fighter{ID: "b", X: math.Round(l.X), Y: 700, HP: 500, MaxHP: 500, Alive: true}
	shell, _ := findWeapon(WeaponShell)
	res := Fire(g, []*Fighter{a, b}, a, shell, 45, 50, 0, func() int { return 0 }) // 0 < may mắn 2: chí mạng
	if len(res.Hits) != 1 || !res.Hits[0].Crit || res.Hits[0].Dmg != 47 {
		t.Fatalf("chí mạng 30×1.1×1.5×0.95: %+v", res.Hits)
	}
	res = Fire(flat(700), []*Fighter{a, b}, a, shell, 45, 50, 0, func() int { return 2 })
	if len(res.Hits) != 1 || res.Hits[0].Crit {
		t.Fatalf("không chí mạng: %+v", res.Hits)
	}
}
