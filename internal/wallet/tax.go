package wallet

import (
	"context"
	"fmt"
)

// Thuế ván game: người thắng xu trong một ván (hoặc một vòng Bầu Cua) bị thu GameTaxPercent% số thắng ròng.
// Người thua không bị thu thêm; bàn chơi vui 0 xu không có thuế. Tiền thuế biến mất, không về tay ai.
const GameTaxPercent = 5

// GameTax là thuế cho số xu thắng ròng win (làm tròn xuống); thắng ≤ 0 thì không thuế.
func GameTax(win int64) int64 {
	if win <= 0 {
		return 0
	}
	return win * GameTaxPercent / 100
}

// Taxes trả thuế của từng người thắng trong deltas (chỉ người có thuế > 0).
func Taxes(deltas map[int64]int64) map[int64]int64 {
	out := map[int64]int64{}
	for id, d := range deltas {
		if t := GameTax(d); t > 0 {
			out[id] = t
		}
	}
	return out
}

// TaxNote là ghi chú sổ cái cho dòng thuế đi kèm một khoản thắng có ghi chú note.
func TaxNote(note string) string {
	return fmt.Sprintf("Thuế %d%% tiền thắng — %s", GameTaxPercent, note)
}

// Settler là thứ ghi kết quả ván vào ví (Wallet.Settle hoặc ví giả trong test).
type Settler interface {
	Settle(ctx context.Context, game, note string, deltas map[int64]int64) (map[int64]int64, error)
}

// SettleTaxed ghi kết quả ván như Settle, rồi thu thuế người thắng bằng một dòng sổ cái riêng TaxNote(note).
// Trả số dư mới và thuế đã thu của từng người (nil nếu ghi thuế lỗi: khi đó không ai bị thu thuế).
func SettleTaxed(ctx context.Context, s Settler, game, note string, deltas map[int64]int64) (map[int64]int64, map[int64]int64, error) {
	balances, err := s.Settle(ctx, game, note, deltas)
	if err != nil {
		return balances, nil, err
	}
	taxes := Taxes(deltas)
	if len(taxes) == 0 {
		return balances, taxes, nil
	}
	neg := map[int64]int64{}
	for id, t := range taxes {
		neg[id] = -t
	}
	after, err := s.Settle(ctx, game, TaxNote(note), neg)
	if err != nil {
		return balances, nil, err
	}
	for id, b := range after {
		balances[id] = b
	}
	return balances, taxes, nil
}
