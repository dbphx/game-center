package wallet

// Damage trừ amount HP trong transaction của game (vd chó giữ nhà cắn kẻ trộm ở nông trại), theo cùng luật máu với
// xe đụng: máu dưới HospitalHP thì nằm viện HospitalStay giây và bị trừ viện phí. Đang nằm viện thì không trừ thêm. Trả số HP thật sự bị trừ.
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
	if left < HospitalHP {
		left, until = 0, t.now+HospitalStay
		if err := t.admit(userID); err != nil {
			return 0, err
		}
	}
	return hp - left, t.setHP(userID, left, until, s.lastHitMs)
}

// admit trừ viện phí HospitalFee khi nhập viện; không đủ xu thì trừ hết số đang có (số dư không âm).
func (t *Tx) admit(userID int64) error {
	b, err := t.balance(userID)
	if err != nil {
		return err
	}
	return t.change(userID, b, -min(b, HospitalFee), "hospital", "Viện phí nằm viện")
}
