// Package farm là game Nông trại của DCity: trồng cây trên các ô đất, nuôi gia súc trong chuồng,
// nuôi cá trong ao. Mọi khoản xu đi qua ví chung (ghi sổ cái game "farm"), thu hoạch / bán được cộng XP.
package farm

// Crop là một loại cây trồng: mua hạt, chờ growSeconds rồi thu hoạch bán được sellPrice.
type Crop struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Level       int    `json:"level"`
	SeedPrice   int64  `json:"seedPrice"`
	GrowSeconds int64  `json:"growSeconds"`
	SellPrice   int64  `json:"sellPrice"`
	XP          int64  `json:"xp"`
}

// Stock là một loại vật nuôi hoặc cá: chiếm units đơn vị sức chứa, lớn sau growSeconds rồi bán.
type Stock struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Level       int    `json:"level"`
	Units       int    `json:"units"`
	Price       int64  `json:"price"`
	GrowSeconds int64  `json:"growSeconds"`
	SellPrice   int64  `json:"sellPrice"`
	XP          int64  `json:"xp"`
}

// Building là chuồng hoặc ao: mua một lần, mở rộng thêm ExpandUnits mỗi lần tới MaxCapacity.
type Building struct {
	Name        string `json:"-"`
	Price       int64  `json:"price"`
	Level       int    `json:"level"`
	Capacity    int    `json:"capacity"`
	ExpandUnits int    `json:"expandUnits"`
	MaxCapacity int    `json:"maxCapacity"`
	expandBase  int64  // giá mở rộng lần thứ n = expandBase × n
}

func (b Building) expandPrice(expansions int) int64 { return b.expandBase * int64(expansions+1) }

var Crops = []Crop{
	{"carrot", "Cà rốt", 1, 50, 120, 90, 2},
	{"corn", "Ngô", 1, 80, 300, 160, 4},
	{"tomato", "Cà chua", 2, 120, 600, 260, 6},
	{"rice", "Lúa", 2, 150, 1200, 360, 8},
	{"strawberry", "Dâu tây", 3, 300, 2700, 750, 15},
	{"watermelon", "Dưa hấu", 4, 500, 7200, 1400, 30},
}

var Animals = []Stock{
	{"chicken", "Gà", 1, 1, 200, 900, 400, 5},
	{"pig", "Heo", 2, 1, 600, 3600, 1300, 12},
	{"sheep", "Cừu", 3, 2, 900, 7200, 2000, 20},
	{"cow", "Bò", 4, 2, 1500, 10800, 3400, 35},
}

var Fish = []Stock{
	{"tilapia", "Cá rô phi", 1, 1, 100, 600, 220, 3},
	{"catfish", "Cá trê", 2, 1, 250, 1800, 560, 8},
	{"carp", "Cá chép", 3, 2, 500, 3600, 1150, 15},
	{"koi", "Cá Koi", 5, 3, 2000, 14400, 4800, 50},
}

// Ô đất: FreePlots ô miễn phí, mua thêm tới MaxPlots; ô kế tiếp giá PlotBasePrice + PlotStep × (số ô đang có − FreePlots).
const (
	FreePlots     = 6
	MaxPlots      = 48
	PlotBasePrice = 10000
	PlotStep      = 2000
)

func plotPrice(have int) int64 { return PlotBasePrice + PlotStep*int64(have-FreePlots) }

const (
	placeBarn = "barn"
	placePond = "pond"
)

var Buildings = map[string]Building{
	placeBarn: {Name: "chuồng", Price: 3000, Level: 2, Capacity: 5, ExpandUnits: 5, MaxCapacity: 30, expandBase: 2000},
	placePond: {Name: "ao cá", Price: 2500, Level: 1, Capacity: 5, ExpandUnits: 5, MaxCapacity: 30, expandBase: 1500},
}

// stockOf là danh mục con nuôi được ở mỗi nơi.
func stockOf(place string) []Stock {
	if place == placeBarn {
		return Animals
	}
	return Fish
}

func findCrop(id string) (Crop, bool) {
	for _, c := range Crops {
		if c.ID == id {
			return c, true
		}
	}
	return Crop{}, false
}

func findStock(place, id string) (Stock, bool) {
	for _, s := range stockOf(place) {
		if s.ID == id {
			return s, true
		}
	}
	return Stock{}, false
}

// Catalog là danh mục gửi cho giao diện (GET /api/farm/catalog).
func Catalog() map[string]any {
	return map[string]any{
		"crops":   Crops,
		"animals": Animals,
		"fish":    Fish,
		"plots":   map[string]int{"free": FreePlots, "max": MaxPlots, "basePrice": PlotBasePrice, "step": PlotStep},
		"barn":    Buildings[placeBarn],
		"pond":    Buildings[placePond],
	}
}
