package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Máu (HP) của nhân vật: va chạm xe trên bản đồ bị trừ máu, hết máu phải nằm viện;
// mỗi khu cần một lượng máu tối thiểu mới được vào.
const (
	MaxHP          = 100
	RegenSeconds   = 60   // tự hồi 1 HP mỗi 60 giây (khi không nằm viện)
	CarDamage      = 20   // mỗi lần bị xe đụng
	HitCooldownMs  = 1500 // bỏ qua va chạm dồn dập trong 1.5 giây
	HospitalStay   = 300  // nằm viện 5 phút thì tự xuất viện đầy máu
	DischargePrice = 200  // xu để xuất viện ngay
	HealPerHP      = 2    // xu cho mỗi HP thiếu khi chữa lúc không nằm viện
)

// ZoneMinHP là lượng máu tối thiểu để vào từng khu trên bản đồ.
var ZoneMinHP = map[string]int{"plaza": 0, "bank": 0, "hospital": 0, "shop": 10, "casino": 30, "farm": 40}

// zoneAction là cụm từ trong thông báo thiếu máu của từng khu.
var zoneAction = map[string]string{"shop": "vào Shop thời trang", "casino": "vào Casino", "farm": "làm nông"}

var (
	ErrInHospital = errors.New("bạn đang nằm viện")
	ErrLowHP      = errors.New("không đủ HP")
	ErrFullHP     = errors.New("bạn đang đầy máu")
	ErrBadSource  = errors.New("nguồn sát thương không hợp lệ")
)

// HPError cho biết cần bao nhiêu HP để vào một khu; errors.Is(err, ErrLowHP) vẫn đúng.
type HPError struct {
	Need int
	Zone string
}

func (e *HPError) Error() string {
	action := zoneAction[e.Zone]
	if action == "" {
		action = "vào khu này"
	}
	return fmt.Sprintf("cần ít nhất %d HP để %s", e.Need, action)
}
func (e *HPError) Is(target error) bool { return target == ErrLowHP }

// CheckZone kiểm tra người chơi có được vào khu không (đủ máu, không nằm viện).
func CheckZone(u User, zone string) error {
	if u.HospitalUntil > 0 {
		return ErrInHospital
	}
	if need := ZoneMinHP[zone]; u.HP < need {
		return &HPError{Need: need, Zone: zone}
	}
	return nil
}

// hpState là phần dữ liệu máu lưu trong bảng users.
type hpState struct {
	hp            int
	hpAt          int64 // unix lúc lưu hp; 0 = chưa tính lần nào (coi như đầy)
	hospitalUntil int64
	lastHitMs     int64
}

// current tính máu hiện tại (lười, không ghi DB): tự hồi theo thời gian, đang nằm viện thì
// máu tăng đều từ 0 lên đầy tới lúc xuất viện, hết hạn thì coi như đã xuất viện đầy máu.
func (s hpState) current(now int64) (hp int, hospitalUntil int64) {
	if s.hospitalUntil > 0 {
		if now >= s.hospitalUntil {
			return MaxHP, 0
		}
		elapsed := now - (s.hospitalUntil - HospitalStay)
		return int(min(MaxHP-1, max(0, elapsed*MaxHP/HospitalStay))), s.hospitalUntil
	}
	if s.hpAt == 0 {
		return MaxHP, 0
	}
	regen := max(0, (now-s.hpAt)/RegenSeconds)
	return int(min(int64(MaxHP), int64(s.hp)+regen)), 0
}

func (t *Tx) hpState(userID int64) (hpState, error) {
	var s hpState
	var hpAt sql.NullInt64
	err := t.QueryRowContext(t.ctx, `SELECT hp, hp_at, hospital_until, last_hit_ms FROM users WHERE id = ?`, userID).
		Scan(&s.hp, &hpAt, &s.hospitalUntil, &s.lastHitMs)
	if errors.Is(err, sql.ErrNoRows) {
		return s, ErrNotFound
	}
	s.hpAt = hpAt.Int64
	return s, err
}

func (t *Tx) setHP(userID int64, hp int, hospitalUntil, lastHitMs int64) error {
	_, err := t.ExecContext(t.ctx, `UPDATE users SET hp = ?, hp_at = ?, hospital_until = ?, last_hit_ms = ? WHERE id = ?`,
		hp, t.now, hospitalUntil, lastHitMs, userID)
	return err
}

// Hit trừ máu khi nhân vật bị xe đụng trên bản đồ. Va chạm dồn dập (dưới 1.5 giây) bị bỏ qua;
// hết máu thì phải nằm viện.
func (w *Wallet) Hit(ctx context.Context, id int64, source string) (User, error) {
	if source != "car" {
		return User{}, ErrBadSource
	}
	t, err := w.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer t.Rollback()
	s, err := t.hpState(id)
	if err != nil {
		return User{}, err
	}
	hp, until := s.current(t.now)
	if until > 0 {
		return User{}, ErrInHospital
	}
	nowMs := w.now().UnixMilli()
	if nowMs-s.lastHitMs < HitCooldownMs {
		t.Rollback() // nhả kết nối trước khi đọc user (database chỉ có một kết nối)
		return w.User(ctx, id)
	}
	hp -= CarDamage
	if hp <= 0 {
		hp, until = 0, t.now+HospitalStay
	}
	if err := t.setHP(id, hp, until, nowMs); err != nil {
		return User{}, err
	}
	if err := t.Commit(); err != nil {
		return User{}, err
	}
	return w.User(ctx, id)
}

// Heal chữa ở bệnh viện: đang nằm viện thì trả viện phí để xuất viện ngay,
// không thì trả theo số HP thiếu. Sau khi chữa luôn đầy máu.
func (w *Wallet) Heal(ctx context.Context, id int64) (User, error) {
	t, err := w.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer t.Rollback()
	s, err := t.hpState(id)
	if err != nil {
		return User{}, err
	}
	hp, until := s.current(t.now)
	switch {
	case until > 0:
		err = t.Charge(id, DischargePrice, "hospital", "Viện phí xuất viện")
	case hp >= MaxHP:
		return User{}, ErrFullHP
	default:
		missing := MaxHP - hp
		err = t.Charge(id, int64(missing*HealPerHP), "hospital", fmt.Sprintf("Chữa trị %d HP", missing))
	}
	if err != nil {
		return User{}, err
	}
	if err := t.setHP(id, MaxHP, 0, s.lastHitMs); err != nil {
		return User{}, err
	}
	if err := t.Commit(); err != nil {
		return User{}, err
	}
	return w.User(ctx, id)
}

// nowUnix dùng cho tính máu khi đọc user.
func (w *Wallet) nowUnix() int64 { return w.now().Unix() }
