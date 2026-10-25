package wallet

// Các khu làm việc / giải trí (công trường, hồ câu...) tiêu máu của nhân vật trong cùng transaction với tiền công.

// SpendHP trừ amount HP (đã tính tự hồi tới bây giờ) và trả máu còn lại. Đang nằm viện thì ErrInHospital;
// máu phải còn lại ít nhất 1 sau khi trừ, không thì ErrLowHP (làm việc không được làm nhân vật ngất).
func (t *Tx) SpendHP(userID int64, amount int) (int, error) {
	s, err := t.hpState(userID)
	if err != nil {
		return 0, err
	}
	hp, until := s.current(t.now)
	if until > 0 {
		return 0, ErrInHospital
	}
	if amount <= 0 {
		return hp, nil
	}
	if hp <= amount {
		return hp, ErrLowHP
	}
	// Giữ phần điểm hồi lẻ (như khi đổi tư thế) để trừ máu không làm mất máu đang hồi dở; làm việc thì đứng dậy.
	hpAt := t.now
	if s.hpAt > 0 && hp < MaxHP {
		hpAt -= s.regenCredit(t.now) % RegenSeconds
	}
	_, err = t.ExecContext(t.ctx, `UPDATE users SET hp = ?, hp_at = ?, sit_until = ? WHERE id = ?`, hp-amount, hpAt, t.now, userID)
	return hp - amount, err
}
