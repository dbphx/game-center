package wallet

import (
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

func TestBuyAndWearHair(t *testing.T) {
	w := open(t)
	u, _ := register(t, w, "lan")
	if u.Hair != DefaultHair || u.Wings != NoWings || !slices.Equal(u.OwnedHair, []string{DefaultHair}) || len(u.OwnedWings) != 0 {
		t.Fatalf("người mới: tóc %q cánh %q có %v %v", u.Hair, u.Wings, u.OwnedHair, u.OwnedWings)
	}
	if _, err := w.BuyItem(ctx, u.ID, KindHair, "afro"); !errors.Is(err, ErrInvalidItem) {
		t.Fatalf("tóc lạ phải lỗi, được %v", err)
	}
	if _, err := w.BuyItem(ctx, u.ID, "hat", "x"); !errors.Is(err, ErrInvalidItem) {
		t.Fatalf("nhóm lạ phải lỗi, được %v", err)
	}
	if _, err := w.BuyItem(ctx, u.ID, KindHair, DefaultHair); !errors.Is(err, ErrAlreadyOwned) {
		t.Fatalf("tóc mặc định có sẵn, được %v", err)
	}
	var lv *LevelError
	if _, err := w.BuyItem(ctx, u.ID, KindHair, "bun"); !errors.As(err, &lv) || lv.Need != 2 {
		t.Fatalf("thiếu level: %v", err)
	}
	if _, err := w.WearItem(ctx, u.ID, KindHair, "ponytail"); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("để tóc chưa mua phải lỗi, được %v", err)
	}

	u, err := w.BuyItem(ctx, u.ID, KindHair, "ponytail")
	if err != nil {
		t.Fatal(err)
	}
	if u.Hair != "ponytail" || u.Balance != StartBalance-300 || !slices.Equal(u.OwnedHair, []string{DefaultHair, "ponytail"}) {
		t.Fatalf("sau khi mua tóc: %+v", u)
	}
	h, _ := w.History(ctx, u.ID, 1)
	if len(h) != 1 || h[0].Delta != -300 || h[0].Game != "shop" || h[0].Note != "Mua Đuôi ngựa" {
		t.Fatalf("sổ cái: %+v", h)
	}
	if _, err := w.BuyItem(ctx, u.ID, KindHair, "ponytail"); !errors.Is(err, ErrAlreadyOwned) {
		t.Fatalf("mua lại phải báo đã có, được %v", err)
	}
	if u, _ = w.WearItem(ctx, u.ID, KindHair, DefaultHair); u.Hair != DefaultHair || u.Balance != StartBalance-300 {
		t.Fatalf("để lại tóc mặc định: %+v", u)
	}
	if u, _ = w.WearItem(ctx, u.ID, KindHair, "ponytail"); u.Hair != "ponytail" {
		t.Fatalf("để lại tóc đã mua: %+v", u)
	}
	// Quần áo đi chung endpoint với kind "outfit".
	if _, err := w.WearItem(ctx, u.ID, KindOutfit, "sport"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.BuyItem(ctx, u.ID, KindOutfit, "hoodie"); !errors.Is(err, ErrLevelTooLow) {
		t.Fatalf("mua áo qua BuyItem vẫn kiểm level, được %v", err)
	}
}

func TestBuyAndRemoveWings(t *testing.T) {
	w := open(t)
	u, _ := register(t, w, "minh")
	w.AwardXP(ctx, map[int64]int64{u.ID: LevelStart(8)})
	if _, err := w.BuyItem(ctx, u.ID, KindWings, "dragon"); !errors.Is(err, ErrNotEnoughCoins) {
		t.Fatalf("cánh rồng 10000 xu phải thiếu xu, được %v", err)
	}
	if got, _ := w.User(ctx, u.ID); got.Balance != StartBalance || got.Wings != NoWings {
		t.Fatalf("mua thất bại không được trừ xu: %+v", got)
	}
	if _, err := w.WearItem(ctx, u.ID, KindWings, "angel"); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("đeo cánh chưa mua phải lỗi, được %v", err)
	}
	if _, err := w.Settle(ctx, "hub", "nạp thử", map[int64]int64{u.ID: 3000}); err != nil {
		t.Fatal(err)
	}
	u, err := w.BuyItem(ctx, u.ID, KindWings, "angel")
	if err != nil {
		t.Fatal(err)
	}
	if u.Wings != "angel" || u.Balance != StartBalance+3000-2500 || !slices.Equal(u.OwnedWings, []string{"angel"}) {
		t.Fatalf("sau khi mua cánh: %+v", u)
	}
	if u, err = w.WearItem(ctx, u.ID, KindWings, NoWings); err != nil || u.Wings != NoWings || !slices.Equal(u.OwnedWings, []string{"angel"}) {
		t.Fatalf("tháo cánh: %+v %v", u, err)
	}
	if u, _ = w.WearItem(ctx, u.ID, KindWings, "angel"); u.Wings != "angel" {
		t.Fatalf("đeo lại cánh: %+v", u)
	}
	// Tóc "" không phải là tháo tóc.
	if _, err := w.WearItem(ctx, u.ID, KindHair, ""); !errors.Is(err, ErrInvalidItem) {
		t.Fatalf("tóc rỗng phải lỗi, được %v", err)
	}
}

// dropV6 bỏ tóc, cánh và bảng user_items để đưa database về dạng trước v6.
const dropV6 = `DROP TABLE user_items; ALTER TABLE users DROP COLUMN hair; ALTER TABLE users DROP COLUMN wings;`

// Người chơi từ database v5 để tóc mặc định, không cánh, vẫn giữ trang phục.
func TestMigrateFromV5DefaultHair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v5.db")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := register(t, w, "binh")
	w.Close()

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(dropV7 + dropV6 + `UPDATE users SET outfit = 'sport'; PRAGMA user_version = 5;`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}

	w, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	got, err := w.User(ctx, u.ID)
	if err != nil || got.Hair != DefaultHair || got.Wings != NoWings || got.Outfit != "sport" ||
		!slices.Equal(got.OwnedHair, []string{DefaultHair}) || len(got.OwnedWings) != 0 || got.Balance != StartBalance {
		t.Fatalf("sau nâng cấp v6: %+v %v", got, err)
	}
	if got, err = w.BuyItem(ctx, u.ID, KindHair, "short_spiky"); err != nil || got.Hair != "short_spiky" {
		t.Fatalf("mua tóc sau nâng cấp: %+v %v", got, err)
	}
}

func TestBuyAndWearFace(t *testing.T) {
	w := open(t)
	u, _ := register(t, w, "hoa")
	if u.Face != DefaultFace || !slices.Equal(u.OwnedFace, []string{DefaultFace}) {
		t.Fatalf("người mới: mặt %q có %v", u.Face, u.OwnedFace)
	}
	var lv *LevelError
	if _, err := w.BuyItem(ctx, u.ID, KindFace, "cool"); !errors.As(err, &lv) || lv.Need != 2 {
		t.Fatalf("thiếu level: %v", err)
	}
	if _, err := w.WearItem(ctx, u.ID, KindFace, "wink"); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("dùng mặt chưa mua phải lỗi, được %v", err)
	}
	u, err := w.BuyItem(ctx, u.ID, KindFace, "wink")
	if err != nil {
		t.Fatal(err)
	}
	if u.Face != "wink" || u.Balance != StartBalance-300 || !slices.Equal(u.OwnedFace, []string{DefaultFace, "wink"}) {
		t.Fatalf("sau khi mua mặt: %+v", u)
	}
	if u, _ = w.WearItem(ctx, u.ID, KindFace, DefaultFace); u.Face != DefaultFace {
		t.Fatalf("về mặt mặc định: %+v", u)
	}
	// Mua mặt không đụng tới tóc / cánh.
	if u.Hair != DefaultHair || u.Wings != NoWings {
		t.Fatalf("tóc / cánh bị đổi: %+v", u)
	}
}

// dropV7 bỏ cột gương mặt để đưa database về dạng v6.
const dropV7 = `ALTER TABLE users DROP COLUMN face;`

// Người chơi từ database v6 giữ tóc / cánh đã có và được mặt mặc định.
func TestMigrateFromV6DefaultFace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v6.db")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := register(t, w, "binh")
	if _, err := w.BuyItem(ctx, u.ID, KindHair, "ponytail"); err != nil {
		t.Fatal(err)
	}
	w.Close()

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(dropV7 + `PRAGMA user_version = 6;`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}

	w, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	got, err := w.User(ctx, u.ID)
	if err != nil || got.Face != DefaultFace || !slices.Equal(got.OwnedFace, []string{DefaultFace}) ||
		got.Hair != "ponytail" || !slices.Equal(got.OwnedHair, []string{DefaultHair, "ponytail"}) {
		t.Fatalf("sau nâng cấp v7: %+v %v", got, err)
	}
	if got, err = w.BuyItem(ctx, u.ID, KindFace, "smile"); err != nil || got.Face != "smile" {
		t.Fatalf("mua mặt sau nâng cấp: %+v %v", got, err)
	}
}
