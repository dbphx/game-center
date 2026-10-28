package farm

// ---------- Lớn và cho sản phẩm chỉ trong lúc no ----------
//
// Luật đói: mỗi chu kỳ chỉ cần cho ăn MỘT lần — ăn xong no tới hết chu kỳ đó. Con mới mua đang đói; bữa đầu
// phủ cả lúc lớn lẫn lứa trứng / sữa / lông đầu tiên (con không cho sản phẩm thì no tới lúc lớn, chờ lấy thịt).
// Sản phẩm có rồi mà chưa lấy thì chưa cần ăn; lấy xong thì đói, cho ăn một lần là ra lứa kế. Vd gà: mua → đói →
// ăn → lớn và đẻ trứng → lấy trứng → đói → ăn → đẻ trứng...

// feedSlack: phần no thiếu vài giây so với hết chu kỳ (đồng hồ lệch, làm tròn) thì coi như đã ăn đủ chu kỳ này.
const feedSlack = 5

// cycleSeconds là độ dài chu kỳ hiện tại của con: chưa lớn thì lớn + lứa sản phẩm đầu (nếu có), đã lớn thì một
// lứa sản phẩm (Every); 0 nếu đã lớn mà không cho sản phẩm (chỉ còn lấy thịt / thu cá).
func cycleSeconds(st Stock, grown bool) int64 {
	every := int64(0)
	if p := yields[st.ID].Product; p != nil {
		every = p.Every
	}
	if !grown {
		return st.GrowSeconds + every
	}
	return every
}

// fullFor là số giây no theo luật cũ (trước khi đổi sang mỗi chu kỳ ăn một lần) — chỉ còn dùng khi nâng cấp
// database từ bản lớn theo đồng hồ.
func fullFor(kind string) int64 {
	switch kind {
	case "chicken", "duck":
		return 20 * 60
	case "pig":
		return 30 * 60
	case "sheep", "cow":
		return 40 * 60
	}
	return 20 * 60 // cá
}

// settle cộng tiến độ từ progressAt tới now, chỉ tính phần thời gian còn no (≤ fedUntil):
// lớn trước, đủ GrowSeconds rồi thì phần dư dồn sang sản phẩm (tối đa 1 món chờ lấy).
func (a *Animal) settle(st Stock, progressAt, now int64) {
	span := max(0, min(now, a.FedUntil)-progressAt)
	if grow := min(span, max(0, st.GrowSeconds-a.growSec)); grow > 0 {
		a.growSec += grow
		span -= grow
		if a.growSec >= st.GrowSeconds {
			a.ReadyAt = progressAt + grow // lúc vừa lớn
		}
	}
	a.Grown = a.growSec >= st.GrowSeconds
	if !a.Grown {
		a.ReadyAt = 0
	}
	if p := yields[st.ID].Product; p != nil && a.Grown {
		a.productSec = min(p.Every, a.productSec+span)
	}
}

// derive tính các trường hiển thị từ tiến độ đã cộng tới now.
func (a *Animal) derive(st Stock, now int64) {
	a.FullFor = cycleSeconds(st, a.Grown)
	a.Hungry = now >= a.FedUntil
	a.Progress = min(1, float64(a.growSec)/float64(st.GrowSeconds))
	fedLeft := max(0, a.FedUntil-now)
	p := yields[st.ID].Product
	// Phần còn lại của chu kỳ hiện tại (0: đã lớn mà không cho sản phẩm, hoặc sản phẩm đang chờ lấy).
	a.cycleLeft = 0
	if !a.Grown {
		a.cycleLeft = st.GrowSeconds - a.growSec
	}
	if p != nil {
		a.cycleLeft += max(0, p.Every-a.productSec)
	}
	if !a.Grown {
		if left := st.GrowSeconds - a.growSec; left <= fedLeft {
			a.ReadyAt = now + left
			if p != nil && left+p.Every <= fedLeft {
				a.ProductReadyAt = a.ReadyAt + p.Every
			}
		} else {
			a.Paused = true
		}
		return
	}
	if p == nil {
		return
	}
	a.ProductProgress = float64(a.productSec) / float64(p.Every)
	a.ProductReady = a.productSec >= p.Every
	switch left := p.Every - a.productSec; {
	case a.ProductReady:
		a.ProductReadyAt = now
	case left <= fedLeft:
		a.ProductReadyAt = now + left
	default:
		a.Paused = true
	}
}

// needsFood: cho ăn còn có ích — chưa lớn, hoặc đã lớn mà đang chờ ra sản phẩm.
func (a Animal) needsFood() bool {
	return !a.Grown || (yields[a.Kind].Product != nil && !a.ProductReady)
}

// canFeed: còn cần ăn và phần no chưa phủ hết chu kỳ hiện tại — tức là chưa ăn trong chu kỳ này
// (con no theo luật cũ, chưa đủ tới hết chu kỳ, cũng cho ăn được một lần để no tới hết chu kỳ).
func (a Animal) canFeed(now int64) bool {
	return a.needsFood() && a.cycleLeft > 0 && a.FedUntil+feedSlack < now+a.cycleLeft
}

// fedUntilAfterMeal là lúc hết no nếu cho ăn bây giờ: hết chu kỳ hiện tại.
func (a Animal) fedUntilAfterMeal(now int64) int64 {
	return now + a.cycleLeft
}
