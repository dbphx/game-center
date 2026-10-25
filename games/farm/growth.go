package farm

// ---------- Lớn và cho sản phẩm chỉ trong lúc no ----------

// fullFor là số giây một con no sau mỗi lần ăn (con mới mua cũng no sẵn chừng ấy).
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
	a.FullFor = fullFor(st.ID)
	a.Hungry = now >= a.FedUntil
	a.Progress = min(1, float64(a.growSec)/float64(st.GrowSeconds))
	fedLeft := max(0, a.FedUntil-now)
	p := yields[st.ID].Product
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

// canFeed: đói, hoặc phần no còn lại dưới 1/4 FullFor (cho ăn thêm để không bị gián đoạn).
func (a Animal) canFeed(now int64) bool {
	return a.FedUntil-now < a.FullFor/4
}
