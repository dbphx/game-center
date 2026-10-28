package wallet

// Damage trừ amount HP trong transaction của game (vd chó giữ nhà cắn kẻ trộm ở nông trại), theo cùng luật máu với
// xe đụng: hết máu thì nằm viện HospitalStay giây. Đang nằm viện thì không trừ thêm. Trả số HP thật sự bị trừ.
func (t *Tx) Damage(userID int64, amount int) (int, error) {
	s, err := t.hpState(userID)
	if err != nil {
		return 0, err
	}
	hp, until := s.current(t.now)
	if until > 0 || amount <= 0 {
		return 0, nil
	}
	left := hp - amount
	if left <= 0 {
		left, until = 0, t.now+HospitalStay
	}
	return hp - left, t.setHP(userID, left, until, s.lastHitMs)
}
