package farm

import "strconv"

// ---------- Thuế bán hàng ----------
//
// Bán trong kho và bán con vật đều bị thu thuế, tiền thuế biến mất khỏi nền kinh tế. Thuế tính trên tổng tiền của từng
// món trong một lần bán, làm tròn xuống. Giá trị nông trại (Khu danh vọng) vẫn tính theo giá bán chưa thuế.
const (
	CropTaxPercent  = 10 // cây trồng dưới đất và cây mây
	StockTaxPercent = 5  // trứng, sữa, lông, thịt, cá và con vật bán nguyên con
)

// taxPercent là mức thuế của một món trong kho sản phẩm hoặc một loại vật nuôi.
func taxPercent(item string) int64 {
	if _, ok := findCrop(item); ok {
		return CropTaxPercent
	}
	if _, ok := findCloudCrop(item); ok {
		return CropTaxPercent
	}
	return StockTaxPercent
}

// saleTax là thuế khi bán món item với tổng tiền gross.
func saleTax(item string, gross int64) int64 { return gross * taxPercent(item) / 100 }

// xuText viết số xu có dấu chấm ngăn hàng nghìn: 60018 → "60.018".
func xuText(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := len(s) > 0 && s[0] == '-'
	if neg {
		s = s[1:]
	}
	out := make([]byte, 0, len(s)+len(s)/3)
	for i := range len(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// taxNote là phần ghi chú sổ cái về thuế.
func taxNote(tax int64) string { return ", thuế " + xuText(tax) + " xu" }
