package quests

import (
	"fmt"
	"math"

	"github.com/dbphx/game-center/games/farm"
	"github.com/dbphx/game-center/games/race"
)

// Loại sự kiện (kind) các game gửi qua Record.
const (
	FarmHarvest = "farm_harvest" // thu hoạch ô cây
	FarmPlant   = "farm_plant"   // gieo hạt
	FarmProduct = "farm_product" // lấy trứng / sữa / len
	FarmAnimal  = "farm_animal"  // bán / thu con vật, cá ao
	FarmZombie  = "farm_zombie"  // hạ zombie (đập tay hoặc súng đậu)
	FarmCloud   = "farm_cloud"   // thu hoạch tầng mây
	FarmVisit   = "farm_visit"   // thăm / trộm vườn bạn
	FishCatch   = "fish_catch"   // câu được cá
	RaceFinish  = "race_finish"  // đua xong (không bỏ cuộc)
	RaceWin     = "race_win"     // về nhất
)

// Template là một mẫu nhiệm vụ: số lượng ngày theo cấp (cấp 1 → cấp 30, nội suy tuyến tính), số lượng tuần
// (0 = không có nhiệm vụ tuần loại này).
type Template struct {
	Kind     string
	Name     string // mẫu tên, %d là số lượng
	DailyMin int
	DailyMax int
	Weekly   int
}

// Templates theo bảng trong spec.
var Templates = []Template{
	{FarmHarvest, "Thu hoạch %d ô cây", 10, 30, 150},
	{FarmPlant, "Gieo %d hạt giống", 10, 30, 0},
	{FarmProduct, "Lấy %d trứng / sữa / len", 5, 15, 60},
	{FarmAnimal, "Bán hoặc thu %d con vật / cá", 2, 6, 25},
	{FarmZombie, "Hạ %d zombie", 2, 5, 20},
	{FarmCloud, "Thu hoạch %d cây tầng mây", 1, 3, 10},
	{FarmVisit, "Thăm hoặc trộm vườn bạn %d lần", 3, 3, 15},
	{FishCatch, "Câu được %d con cá", 5, 15, 60},
	{RaceFinish, "Hoàn thành %d cuộc đua", 2, 4, 15},
	{RaceWin, "Về nhất %d cuộc đua", 1, 1, 5},
}

func findTemplate(kind string) (Template, bool) {
	for _, t := range Templates {
		if t.Kind == kind {
			return t, true
		}
	}
	return Template{}, false
}

// dailyTarget là số lượng nhiệm vụ ngày theo cấp người chơi (cấp ≥ 30 lấy mức trần).
func (t Template) dailyTarget(level int) int {
	lv := min(max(level, 1), 30)
	return int(math.Round(float64(t.DailyMin) + float64(t.DailyMax-t.DailyMin)*float64(lv-1)/29))
}

func (t Template) title(target int) string { return fmt.Sprintf(t.Name, target) }

// Phần thưởng.
const (
	DailyCount   = 5   // số nhiệm vụ ngày
	WeeklyCount  = 3   // số nhiệm vụ tuần
	DailyShards  = 1   // mảnh mỗi nhiệm vụ ngày
	DailyXP      = 20  //
	WeeklyShards = 3   // mảnh mỗi nhiệm vụ tuần
	WeeklyXP     = 100 //
	ChestShards  = 1   // rương ngày: 1 mảnh ngẫu nhiên; rương tuần: 1 mảnh độc quyền

	// Phụ kiện nâng cấp xe (ngẫu nhiên đều 3 loại: Piston / Bộ tăng áp / Lốp đua), thêm vào cạnh mảnh.
	DailyParts       = 1 // mỗi nhiệm vụ ngày
	DailyChestParts  = 2 // rương ngày
	WeeklyParts      = 3 // mỗi nhiệm vụ tuần
	WeeklyChestParts = 5 // rương tuần

	RerollCost    = 2000 // xu mỗi lần đổi nhiệm vụ sau lần miễn phí
	RerollsPerDay = 5    // tối đa số lần đổi mỗi ngày (lần đầu miễn phí)
	ExchangeCost  = 3    // 3 mảnh thừa cùng bậc → 1 mảnh bậc trên
)

// Bậc mảnh.
const (
	TierCommon    = "common"
	TierRare      = "rare"
	TierEpic      = "epic"
	TierExclusive = "exclusive"
	TierVip       = "vip" // mảnh chậu Noel VIP: chỉ rơi trong mùa Noel, không đổi lên bậc
)

// PotShardRate: trong mùa Noel, phần nghìn số lần rơi mảnh (ngẫu nhiên) ra mảnh chậu VIP; còn lại theo tỉ lệ mảnh xe.
const PotShardRate = 100

// Tiers theo thứ tự từ thấp tới cao; Rate là tỉ lệ rơi bậc (phần nghìn).
var Tiers = []struct {
	ID   string
	Rate int
}{{TierCommon, 600}, {TierRare, 280}, {TierEpic, 100}, {TierExclusive, 20}}

// Shard là một loại mảnh: id trùng id xe / đồ trang trí của Trường đua.
type Shard struct {
	Item    string
	Name    string
	Tier    string
	Kind    string // "car", "item" hoặc "pot" (chậu Noel VIP của nông trại)
	Need    int    // số mảnh để ghép
	Vehicle string // xe: "car" / "moto"
	Weight  int    // chậu VIP: trọng số rút trong nhóm mảnh chậu
	Speed   int    // chậu VIP: % rút ngắn thời gian chín
}

// shardTier: bậc của từng xe bán bằng xu (Xe Tập Lái không có mảnh).
var shardTier = map[string]string{
	"comet": TierCommon, "moto_dash": TierCommon, "sprint": TierCommon, "bolt": TierCommon, "raptor": TierCommon,
	"viper": TierRare, "moto_storm": TierRare, "zenith": TierRare, "phantom": TierRare, "moto_neon": TierRare,
	"vortex": TierEpic, "apex": TierEpic, "blaze": TierEpic, "nova": TierEpic,
}

var needOf = map[string]int{TierCommon: 6, TierRare: 8, TierEpic: 8}

const (
	exclusiveCarNeed  = 5
	exclusiveItemNeed = 3
)

// potShard: số mảnh và trọng số rơi của từng chậu VIP (rẻ ra nhiều hơn).
var potShard = map[string]struct{ need, weight int }{
	"vip_gift": {4, 55}, "vip_snowman": {6, 30}, "vip_pine": {8, 15},
}

// Shards là mọi loại mảnh, theo bậc rồi theo hạng xe.
var Shards = buildShards()

func buildShards() []Shard {
	var out []Shard
	for _, tier := range []string{TierCommon, TierRare, TierEpic} {
		for _, c := range race.Cars {
			if shardTier[c.ID] == tier {
				out = append(out, Shard{Item: c.ID, Name: c.Name, Tier: tier, Kind: "car", Need: needOf[tier], Vehicle: c.Kind})
			}
		}
	}
	for _, c := range race.ExclusiveCars {
		out = append(out, Shard{Item: c.ID, Name: c.Name, Tier: TierExclusive, Kind: "car", Need: exclusiveCarNeed, Vehicle: c.Kind})
	}
	for _, it := range race.ExclusiveItems {
		out = append(out, Shard{Item: it.ID, Name: it.Name, Tier: TierExclusive, Kind: "item", Need: exclusiveItemNeed})
	}
	for _, p := range farm.VipPots() {
		ps := potShard[p.ID]
		out = append(out, Shard{Item: p.ID, Name: p.Name, Tier: TierVip, Kind: "pot", Need: ps.need, Weight: ps.weight, Speed: p.SpeedPct})
	}
	return out
}

func findShard(item string) (Shard, bool) {
	for _, s := range Shards {
		if s.Item == item {
			return s, true
		}
	}
	return Shard{}, false
}

func shardsOf(tier string) []Shard {
	var out []Shard
	for _, s := range Shards {
		if s.Tier == tier {
			out = append(out, s)
		}
	}
	return out
}

// nextTier là bậc trên của tier ("" nếu đã cao nhất). Bậc Sử thi đổi lên Độc quyền; Độc quyền không đổi tiếp.
func nextTier(tier string) string {
	for i, t := range Tiers {
		if t.ID == tier && i+1 < len(Tiers) {
			return Tiers[i+1].ID
		}
	}
	return ""
}
