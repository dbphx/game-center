package wallet

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

var ctx = context.Background()

func open(t *testing.T) *Wallet {
	t.Helper()
	w, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	return w
}

func register(t *testing.T, w *Wallet, username string) (User, string) {
	t.Helper()
	u, token, err := w.Register(ctx, username, "matkhau123", Profile{})
	if err != nil {
		t.Fatal(err)
	}
	return u, token
}

func TestRegisterAndLogin(t *testing.T) {
	w := open(t)
	u, token, err := w.Register(ctx, "  An.Nguyen ", "matkhau123", Profile{Name: "  An   Nguyễn  ", Gender: "female", Outfit: "sport"})
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "an.nguyen" || u.Name != "An Nguyễn" || u.Balance != StartBalance || !u.CanDaily ||
		u.Gender != "female" || u.Outfit != "sport" || u.Level != 1 || u.NextXP != 100 ||
		!slices.Equal(u.Owned, []string{"casual", "sport"}) {
		t.Fatalf("tài khoản mới sai: %+v", u)
	}
	if got, err := w.UserByToken(ctx, token); err != nil || got.ID != u.ID {
		t.Fatalf("token sau đăng ký: %+v %v", got, err)
	}

	// Đăng nhập không phân biệt hoa thường, mỗi lần đăng nhập là một phiên riêng.
	_, token2, err := w.Login(ctx, "AN.NGUYEN", "matkhau123")
	if err != nil || token2 == token {
		t.Fatalf("đăng nhập: %v", err)
	}
	if _, _, err := w.Login(ctx, "an.nguyen", "sai"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("sai mật khẩu phải ErrBadCredentials, được %v", err)
	}
	if _, _, err := w.Login(ctx, "khongco", "matkhau123"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("không có tài khoản phải ErrBadCredentials, được %v", err)
	}

	// Đăng xuất một phiên không ảnh hưởng phiên khác.
	if err := w.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := w.UserByToken(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("token đã đăng xuất phải hết hạn, được %v", err)
	}
	if _, err := w.UserByToken(ctx, token2); err != nil {
		t.Fatalf("phiên còn lại phải dùng được: %v", err)
	}
	if u, _ := w.Rename(ctx, u.ID, "Bình"); u.Name != "Bình" {
		t.Fatalf("đổi tên sai: %+v", u)
	}
}

func TestRegisterValidation(t *testing.T) {
	w := open(t)
	register(t, w, "anh")
	cases := map[string]struct {
		username, password string
		want               error
	}{
		"trùng (khác hoa thường)": {"ANH", "matkhau123", ErrUsernameTaken},
		"quá ngắn":                {"ab", "matkhau123", ErrInvalidUsername},
		"có dấu":                  {"nguyễn", "matkhau123", ErrInvalidUsername},
		"có dấu cách":             {"an nguyen", "matkhau123", ErrInvalidUsername},
		"mật khẩu ngắn":           {"binh", "12345", ErrWeakPassword},
	}
	for name, tc := range cases {
		if _, _, err := w.Register(ctx, tc.username, tc.password, Profile{}); !errors.Is(err, tc.want) {
			t.Errorf("%s: muốn %v, được %v", name, tc.want, err)
		}
	}
}

func TestChangePassword(t *testing.T) {
	w := open(t)
	u, _ := register(t, w, "binh")
	if err := w.ChangePassword(ctx, u.ID, "sai", "moi123456"); err == nil {
		t.Fatal("mật khẩu cũ sai phải lỗi")
	}
	if err := w.ChangePassword(ctx, u.ID, "matkhau123", "moi123456"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.Login(ctx, "binh", "moi123456"); err != nil {
		t.Fatalf("đăng nhập bằng mật khẩu mới: %v", err)
	}
}

func TestDailyOncePerVietnamDay(t *testing.T) {
	w := open(t)
	clock := time.Date(2026, 10, 24, 16, 0, 0, 0, time.UTC) // 23h giờ VN
	w.now = func() time.Time { return clock }
	u, _ := register(t, w, "binh")

	if u, err := w.ClaimDaily(ctx, u.ID); err != nil || u.Balance != StartBalance+DailyBonus || u.CanDaily {
		t.Fatalf("nhận lần đầu: %+v %v", u, err)
	}
	if _, err := w.ClaimDaily(ctx, u.ID); !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("nhận lần hai trong ngày phải lỗi, được %v", err)
	}
	clock = clock.Add(2 * time.Hour) // 1h sáng hôm sau giờ VN
	if u, err := w.ClaimDaily(ctx, u.ID); err != nil || u.Balance != StartBalance+2*DailyBonus {
		t.Fatalf("sang ngày mới phải nhận được: %+v %v", u, err)
	}
}

func TestSettleClampsAtZeroAndRecordsLedger(t *testing.T) {
	w := open(t)
	a, _ := register(t, w, "binh")
	b, _ := register(t, w, "chi")

	bal, err := w.Settle(ctx, "tienlen", "ván 1", map[int64]int64{a.ID: 300, b.ID: -1500})
	if err != nil {
		t.Fatal(err)
	}
	if bal[a.ID] != 1300 || bal[b.ID] != 0 {
		t.Fatalf("số dư sai: %v", bal)
	}
	h, _ := w.History(ctx, b.ID, 10)
	if len(h) != 2 || h[0].Delta != -1000 || h[0].BalanceAfter != 0 || h[0].Game != "tienlen" || h[1].Note != "Quà chào mừng" {
		t.Fatalf("sổ cái sai: %+v", h)
	}
	if _, err := w.Settle(ctx, "tienlen", "x", map[int64]int64{999: 10}); err == nil {
		t.Fatal("người không tồn tại phải lỗi")
	}
	if u, _ := w.User(ctx, a.ID); u.Balance != 1300 {
		t.Fatalf("lỗi phải rollback cả transaction, số dư %d", u.Balance)
	}
}

// Database từ bản cũ (tài khoản khách, token nằm trong bảng users) phải được nâng cấp
// mà người đang chơi không mất xu, không bị đăng xuất, và đặt được tên đăng nhập.
func TestMigrateFromGuestSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, token_hash TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
	balance INTEGER NOT NULL CHECK (balance >= 0), last_daily TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL);
CREATE TABLE ledger (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL REFERENCES users(id), delta INTEGER NOT NULL,
	balance_after INTEGER NOT NULL, game TEXT NOT NULL, note TEXT NOT NULL, created_at INTEGER NOT NULL);
CREATE INDEX ledger_user ON ledger(user_id, id);
INSERT INTO users VALUES (1, ?, 'Khách cũ', 1700, '', 0);
INSERT INTO ledger VALUES (1, 1, 1000, 1000, 'hub', 'Quà chào mừng', 0), (2, 1, 700, 1700, 'tienlen', 'ván 1', 0);`,
		hashToken("token-cu"))
	db.Close()
	if err != nil {
		t.Fatal(err)
	}

	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	u, err := w.UserByToken(ctx, "token-cu")
	if err != nil || u.Name != "Khách cũ" || u.Balance != 1700 || u.Username != "" || u.Gender != "male" || u.Level != 1 || u.HP != MaxHP {
		t.Fatalf("khách cũ sau nâng cấp: %+v %v", u, err)
	}
	if h, _ := w.History(ctx, u.ID, 10); len(h) != 2 {
		t.Fatalf("sổ cái phải còn nguyên: %+v", h)
	}
	if u, err = w.SetCredentials(ctx, u.ID, "khachcu", "matkhau123"); err != nil || u.Username != "khachcu" {
		t.Fatalf("đặt tên đăng nhập: %+v %v", u, err)
	}
	if _, err := w.SetCredentials(ctx, u.ID, "khac", "matkhau123"); !errors.Is(err, ErrHasCredentials) {
		t.Fatalf("đặt lần hai phải lỗi, được %v", err)
	}
	if got, _, err := w.Login(ctx, "khachcu", "matkhau123"); err != nil || got.Balance != 1700 {
		t.Fatalf("đăng nhập sau khi đặt: %+v %v", got, err)
	}
	// Tài khoản mới sau nâng cấp vẫn tạo bình thường và có id tiếp theo.
	if nu, _, err := w.Register(ctx, "moi", "matkhau123", Profile{}); err != nil || nu.ID != 2 {
		t.Fatalf("đăng ký sau nâng cấp: %+v %v", nu, err)
	}
}

func TestLevels(t *testing.T) {
	cases := map[int64]int{0: 1, 99: 1, 100: 2, 299: 2, 300: 3, 600: 4, 1000: 5}
	for xp, want := range cases {
		if got := LevelFor(xp); got != want {
			t.Errorf("%d XP: muốn level %d, được %d", xp, want, got)
		}
	}
}

func TestCharacterAndXP(t *testing.T) {
	w := open(t)
	if _, _, err := w.Register(ctx, "binh", "matkhau123", Profile{Gender: "alien"}); !errors.Is(err, ErrInvalidCharacter) {
		t.Fatalf("giới tính lạ phải lỗi, được %v", err)
	}
	if _, _, err := w.Register(ctx, "binh", "matkhau123", Profile{Outfit: "aodai"}); !errors.Is(err, ErrInvalidCharacter) {
		t.Fatalf("tạo nhân vật chỉ được chọn trang phục starter, được %v", err)
	}
	u, _ := register(t, w, "binh")
	if u.Gender != "male" || u.Outfit != "casual" {
		t.Fatalf("nhân vật mặc định sai: %+v", u)
	}
	if u, _ = w.WearOutfit(ctx, u.ID, "sport"); u.Outfit != "sport" || u.Gender != "male" {
		t.Fatalf("mặc trang phục starter: %+v", u)
	}
	if _, err := w.WearOutfit(ctx, u.ID, "aodai"); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("mặc trang phục chưa có phải lỗi, được %v", err)
	}
	if _, err := w.WearOutfit(ctx, u.ID, "bikini"); !errors.Is(err, ErrInvalidCharacter) {
		t.Fatalf("trang phục lạ phải lỗi, được %v", err)
	}
	levels, err := w.AwardXP(ctx, map[int64]int64{u.ID: 120})
	if err != nil || levels[u.ID] != 2 {
		t.Fatalf("cộng XP: %v %v", levels, err)
	}
	if u, _ = w.User(ctx, u.ID); u.XP != 120 || u.Level != 2 || u.LevelXP != 100 || u.NextXP != 300 {
		t.Fatalf("level sau khi cộng XP: %+v", u)
	}
}

func TestBuyOutfit(t *testing.T) {
	w := open(t)
	u, _ := register(t, w, "binh")

	// Level 1 chưa mua được hoodie (cần level 2).
	_, err := w.BuyOutfit(ctx, u.ID, "hoodie")
	var lv *LevelError
	if !errors.Is(err, ErrLevelTooLow) || !errors.As(err, &lv) || lv.Need != 2 || err.Error() != "cần level 2 để mua" {
		t.Fatalf("thiếu level: %v", err)
	}
	if _, err := w.BuyOutfit(ctx, u.ID, "bikini"); !errors.Is(err, ErrInvalidCharacter) {
		t.Fatalf("trang phục lạ phải lỗi, được %v", err)
	}
	if _, err := w.BuyOutfit(ctx, u.ID, "sport"); !errors.Is(err, ErrAlreadyOwned) {
		t.Fatalf("starter đã có sẵn, được %v", err)
	}

	// Đủ level (level 6) nhưng thiếu xu cho royal (5000).
	w.AwardXP(ctx, map[int64]int64{u.ID: LevelStart(6)})
	if _, err := w.BuyOutfit(ctx, u.ID, "royal"); !errors.Is(err, ErrNotEnoughCoins) {
		t.Fatalf("thiếu xu phải lỗi, được %v", err)
	}
	if got, _ := w.User(ctx, u.ID); got.Balance != StartBalance || slices.Contains(got.Owned, "royal") {
		t.Fatalf("mua thất bại không được trừ xu: %+v", got)
	}

	// Mua thành công: trừ xu, ghi sổ cái, có trong owned, mặc luôn.
	u, err = w.BuyOutfit(ctx, u.ID, "hoodie")
	if err != nil {
		t.Fatal(err)
	}
	if u.Balance != StartBalance-800 || u.Outfit != "hoodie" || !slices.Equal(u.Owned, []string{"casual", "sport", "hoodie"}) {
		t.Fatalf("sau khi mua: %+v", u)
	}
	h, _ := w.History(ctx, u.ID, 1)
	if len(h) != 1 || h[0].Delta != -800 || h[0].Game != "shop" || h[0].Note != "Mua trang phục Áo hoodie" || h[0].BalanceAfter != u.Balance {
		t.Fatalf("sổ cái sau khi mua: %+v", h)
	}
	if _, err := w.BuyOutfit(ctx, u.ID, "hoodie"); !errors.Is(err, ErrAlreadyOwned) {
		t.Fatalf("mua lại phải báo đã có, được %v", err)
	}
	// Đổi qua lại giữa các món đã có thì miễn phí; giới tính giữ nguyên.
	if u, _ = w.WearOutfit(ctx, u.ID, "casual"); u.Outfit != "casual" || u.Balance != StartBalance-800 || u.Gender != "male" {
		t.Fatalf("mặc lại starter: %+v", u)
	}
	if u, _ = w.WearOutfit(ctx, u.ID, "hoodie"); u.Outfit != "hoodie" {
		t.Fatalf("mặc lại món đã mua: %+v", u)
	}
}

// Database v3 (trước khi có shop): người chơi giữ trang phục đang mặc dù trước đây chọn miễn phí.
func TestMigrateFromV3KeepsWornOutfit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v3.db")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := register(t, w, "binh")
	w.Close()

	// Đưa database về dạng v3: bỏ bảng user_outfits, đang mặc áo dài.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`DROP TABLE user_outfits; UPDATE users SET outfit = 'aodai';` + dropV6 + dropV5 + `PRAGMA user_version = 3;`)
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
	if err != nil || got.Outfit != "aodai" || !slices.Equal(got.Owned, []string{"casual", "sport", "aodai"}) {
		t.Fatalf("sau nâng cấp v4: %+v %v", got, err)
	}
	if _, err := w.WearOutfit(ctx, u.ID, "aodai"); err != nil {
		t.Fatalf("vẫn mặc được áo dài: %v", err)
	}
}

// dropV5 bỏ các cột máu để đưa database về dạng trước v5.
const dropV5 = `ALTER TABLE users DROP COLUMN hp; ALTER TABLE users DROP COLUMN hp_at;
ALTER TABLE users DROP COLUMN hospital_until; ALTER TABLE users DROP COLUMN last_hit_ms;`

// Người chơi từ database v4 được đầy máu, không nằm viện.
func TestMigrateFromV4FullHP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v4.db")
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
	_, err = db.Exec(dropV6 + dropV5 + `PRAGMA user_version = 4;`)
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
	if err != nil || got.HP != MaxHP || got.HospitalUntil != 0 || got.Balance != StartBalance {
		t.Fatalf("sau nâng cấp v5: %+v %v", got, err)
	}
	if got, err = w.Hit(ctx, u.ID, "car"); err != nil || got.HP != MaxHP-CarDamage {
		t.Fatalf("va chạm sau nâng cấp: %+v %v", got, err)
	}
}
