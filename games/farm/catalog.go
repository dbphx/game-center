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

// Cây ngắn lãi / giờ cao cho người chơi liên tục; cây dài (4–12 giờ) cho người treo máy. Cùng thời gian thì
// cây level cao lãi hơn.
var Crops = []Crop{
	{"water_spinach", "Rau muống", 1, 30, 60, 52, 1},
	{"carrot", "Cà rốt", 1, 50, 120, 90, 2},
	{"corn", "Ngô", 1, 80, 300, 160, 4},
	{"tomato", "Cà chua", 2, 120, 600, 260, 6},
	{"sweet_potato", "Khoai lang", 2, 100, 900, 290, 5},
	{"rice", "Lúa", 2, 150, 1200, 360, 8},
	{"cabbage", "Bắp cải", 3, 180, 1800, 520, 10},
	{"strawberry", "Dâu tây", 3, 300, 2700, 750, 15},
	{"chili", "Ớt", 3, 220, 3600, 820, 14},
	{"eggplant", "Cà tím", 4, 260, 1500, 700, 9},
	{"watermelon", "Dưa hấu", 4, 500, 7200, 1400, 30},
	{"pineapple", "Dứa", 5, 400, 3600, 1250, 18},
	{"pumpkin", "Bí ngô", 5, 600, 14400, 2600, 45},
	{"dragon_fruit", "Thanh long", 6, 800, 21600, 4400, 70},
	{"grape", "Nho", 7, 1000, 10800, 3700, 55},
	{"durian", "Sầu riêng", 8, 2000, 43200, 12800, 150},
}

// Poultry là gia cầm thả rông trong sân (đi quanh trang trại), không nuôi trong chuồng.
var Poultry = []Stock{
	{"chicken", "Gà", 1, 1, 200, 900, 400, 5},
	{"duck", "Vịt", 1, 1, 250, 1200, 520, 6},
}

var Animals = []Stock{
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
	placeYard = "yard" // sân gia cầm: có sẵn cho mọi người, không cần mua
)

var Buildings = map[string]Building{
	placeBarn: {Name: "chuồng", Price: 3000, Level: 1, Capacity: 5, ExpandUnits: 5, MaxCapacity: 30, expandBase: 20000},
	placeYard: {Name: "sân gia cầm", Price: 0, Level: 1, Capacity: 10, ExpandUnits: 5, MaxCapacity: 40, expandBase: 10000},
	placePond: {Name: "ao cá", Price: 2500, Level: 1, Capacity: 5, ExpandUnits: 5, MaxCapacity: 30, expandBase: 15000},
}

const (
	storeSeeds    = "seeds"
	storeProducts = "products"
)

// StoreSpec là một kho: mỗi món chiếm 1 đơn vị; mở rộng thêm ExpandUnits mỗi lần tới MaxCapacity,
// lần mở rộng thứ n giá ExpandBase × n.
type StoreSpec struct {
	Name        string `json:"-"`
	Capacity    int    `json:"capacity"`
	ExpandUnits int    `json:"expandUnits"`
	MaxCapacity int    `json:"maxCapacity"`
	ExpandBase  int64  `json:"expandBase"`
}

func (s StoreSpec) expandPrice(expansions int) int64 { return s.ExpandBase * int64(expansions+1) }

var Stores = map[string]StoreSpec{
	storeSeeds:    {Name: "kho giống", Capacity: 50, ExpandUnits: 25, MaxCapacity: 300, ExpandBase: 10000},
	storeProducts: {Name: "kho sản phẩm", Capacity: 50, ExpandUnits: 25, MaxCapacity: 500, ExpandBase: 15000},
}

// product trả tên và giá bán của một món trong kho sản phẩm: nông sản, trứng / sữa / lông, thịt,
// cá, hoặc con vật nguyên con (kho từ bản cũ).
func product(item string) (name string, price int64, ok bool) {
	if c, ok := findCrop(item); ok {
		return c.Name, c.SellPrice, true
	}
	for _, y := range yields {
		if y.Product != nil && y.Product.ID == item {
			return y.Product.Name, y.Product.SellPrice, true
		}
		if y.Meat.ID == item {
			return y.Meat.Name, y.Meat.SellPrice, true
		}
	}
	for _, place := range []string{placeBarn, placeYard, placePond} {
		if st, ok := findStock(place, item); ok {
			return st.Name, st.SellPrice, true
		}
	}
	return "", 0, false
}

// stockOf là danh mục con nuôi được ở mỗi nơi.
func stockOf(place string) []Stock {
	switch place {
	case placeBarn:
		return Animals
	case placeYard:
		return Poultry
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
		"animals": withYields(withFeed(Animals, placeBarn)),
		"poultry": withYields(withFeed(Poultry, placeYard)),
		"feeds":   Feeds,
		"yard":    Buildings[placeYard],
		"fish":    withFeed(Fish, placePond),
		"plots":   map[string]int{"free": FreePlots, "max": MaxPlots, "basePrice": PlotBasePrice, "step": PlotStep},
		"barn":    Buildings[placeBarn],
		"pond":    Buildings[placePond],
		"storage": Stores,
	}
}
