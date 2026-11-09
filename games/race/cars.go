// Package race là Trường đua: phòng đua kart 3D 2–6 xe, cược xu, garage mua xe, nâng cấp và đồ trang trí.
//
// Mỗi máy tự mô phỏng xe mình rồi gửi vị trí lên; server kiểm gói (tốc độ, dịch chuyển, điểm kiểm tra, thời gian vòng),
// tự tính vòng / về đích, chạy máy đua và trả xu.
package race

import "math"

const (
	GameID   = "race"
	MaxLevel = 5 // mỗi món nâng cấp tối đa cấp 5

	// Chỉ số xe khởi đầu cấp 0.
	BaseSpeed = 30.0 // m/s
	BaseAccel = 12.0 // m/s²
	BaseGrip  = 0.70 // 0–1

	NitroBoost  = 1.3 // nitro: ×1,3 tốc độ tối đa trong NitroSeconds
	NitroMs     = 2000
	NitroCharge = 2.0 // hệ số tốc độ tích nitro (giao diện nhân vào mức tích gốc): tích nhanh gấp đôi bản đầu

	// Trần: xe mạnh nhất cấp tối đa hơn xe khởi đầu cấp 0 tối đa 20% ở mỗi chỉ số.
	topBonus = 1.2

	// Moto so với ô tô cùng hạng: tăng tốc nhanh hơn, bám đường kém hơn.
	motoAccel = 1.05
	motoGrip  = 0.95

	levelStep = 0.025 // mỗi cấp nâng +2,5% (cấp 5: ×1,125)
)

// Loại xe.
const (
	KindCar  = "car"
	KindMoto = "moto"
)

// CarModel là một mẫu xe trong cửa hàng; Rank 0 là xe khởi đầu miễn phí, hạng tăng theo giá.
type CarModel struct {
	ID    string
	Name  string
	Price int64
	Rank  int
	Kind  string // KindCar hoặc KindMoto
	Level int    // cấp nhân vật cần để mua
	// Exclusive: xe độc quyền, không bán bằng xu, chỉ ghép từ mảnh (nhiệm vụ nông trại).
	Exclusive bool
}

// Cars là các mẫu xe theo thứ tự giá (cũng là thứ tự hạng): xe sau đẹp hơn và chỉ số gốc cao hơn.
var Cars = rankCars([]CarModel{
	{ID: "starter", Name: "Xe Tập Lái", Price: 0, Level: 1},
	{ID: "comet", Name: "Sao Chổi", Price: 60_000, Level: 3},
	{ID: "moto_dash", Name: "Gió Lốc", Price: 90_000, Level: 5, Kind: KindMoto},
	{ID: "sprint", Name: "Tốc Hành", Price: 150_000, Level: 7},
	{ID: "bolt", Name: "Tia Chớp", Price: 240_000, Level: 9},
	{ID: "raptor", Name: "Chim Ưng", Price: 450_000, Level: 12},
	{ID: "viper", Name: "Rắn Lục", Price: 750_000, Level: 15},
	{ID: "moto_storm", Name: "Bão Tố", Price: 900_000, Level: 17, Kind: KindMoto},
	{ID: "zenith", Name: "Thiên Đỉnh", Price: 1_200_000, Level: 19},
	{ID: "phantom", Name: "Bóng Ma", Price: 2_400_000, Level: 22},
	{ID: "moto_neon", Name: "Neon Rider", Price: 3_000_000, Level: 24, Kind: KindMoto},
	{ID: "vortex", Name: "Lốc Xoáy", Price: 3_600_000, Level: 26},
	{ID: "apex", Name: "Đỉnh Cao", Price: 4_800_000, Level: 28},
	{ID: "blaze", Name: "Hoả Tiễn", Price: 5_400_000, Level: 29},
	{ID: "nova", Name: "Siêu Tân Tinh", Price: 6_000_000, Level: 30},
})

// rankCars đánh hạng theo thứ tự trong danh sách và điền loại mặc định là ô tô.
func rankCars(list []CarModel) []CarModel {
	for i := range list {
		list[i].Rank = i
		if list[i].Kind == "" {
			list[i].Kind = KindCar
		}
	}
	return list
}

// ExclusiveCars là xe độc quyền chỉ ghép từ mảnh: chỉ số đúng bằng hạng Siêu Tân Tinh (moto theo quy tắc moto),
// không nằm trong Cars nên không đổi hạng / rankBonus của các xe bán bằng xu.
var ExclusiveCars = exclusiveCars([]CarModel{
	{ID: "x_aurora", Name: "Cực Quang", Kind: KindCar},
	{ID: "x_frost", Name: "Băng Phong", Kind: KindMoto},
})

func exclusiveCars(list []CarModel) []CarModel {
	for i := range list {
		list[i].Rank = len(Cars) - 1
		list[i].Level = 1
		list[i].Exclusive = true
	}
	return list
}

// AllCars là mọi mẫu xe: xe bán bằng xu theo hạng rồi tới xe độc quyền.
func AllCars() []CarModel { return append(append([]CarModel{}, Cars...), ExclusiveCars...) }

func findCar(id string) (CarModel, bool) {
	for _, c := range AllCars() {
		if c.ID == id {
			return c, true
		}
	}
	return CarModel{}, false
}

// Levels là cấp của 3 món nâng cấp: động cơ / ống xả (tốc độ), turbo (tăng tốc), lốp / mâm (bám đường).
type Levels struct {
	Engine int `json:"engine"`
	Turbo  int `json:"turbo"`
	Tires  int `json:"tires"`
}

// Parts là tên 3 món nâng cấp, dùng trong API.
var Parts = []string{"engine", "turbo", "tires"}

func (l Levels) get(part string) int {
	switch part {
	case "engine":
		return l.Engine
	case "turbo":
		return l.Turbo
	case "tires":
		return l.Tires
	}
	return -1
}

// Stats là chỉ số lái; server là nguồn đúng, giao diện dùng để mô phỏng và hiện.
type Stats struct {
	MaxSpeed float64 `json:"maxSpeed"`
	Accel    float64 `json:"accel"`
	Grip     float64 `json:"grip"`
}

// rankBonus: xe hạng cao nhất hơn xe khởi đầu đúng bao nhiêu để cộng cấp 5 (×1,125) thì ≈ ×1,20.
func rankBonus(rank int) float64 {
	top := float64(len(Cars) - 1)
	return 1 + float64(rank)*(topBonus/(1+levelStep*MaxLevel)-1)/top
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// StatsOf là chỉ số của mẫu xe c ở các cấp lv.
// Moto tăng tốc +5%, bám đường −5% so với ô tô cùng hạng; mọi chỉ số vẫn không vượt trần +20%.
func StatsOf(c CarModel, lv Levels) Stats {
	rb := rankBonus(c.Rank)
	accel, grip := BaseAccel*rb*(1+levelStep*float64(lv.Turbo)), BaseGrip*rb*(1+levelStep*float64(lv.Tires))
	if c.Kind == KindMoto {
		accel, grip = accel*motoAccel, grip*motoGrip
	}
	return Stats{
		MaxSpeed: round2(BaseSpeed * rb * (1 + levelStep*float64(lv.Engine))),
		Accel:    round2(math.Min(accel, BaseAccel*topBonus)),
		Grip:     round2(math.Min(1, math.Min(grip, BaseGrip*topBonus))),
	}
}

// upgradePrice là giá nâng một món lên cấp toLevel: giá xe × 10% × cấp (xe khởi đầu tính như 30.000 xu).
func upgradePrice(c CarModel, toLevel int) int64 {
	base := c.Price
	if base == 0 {
		base = 30_000
	}
	return base / 10 * int64(toLevel)
}

// Item là đồ trang trí (không có chỉ số): cánh gió, đèn neon gầm, sơn (miễn phí), decal.
type Item struct {
	ID    string
	Kind  string // spoiler, neon, paint, decal
	Name  string
	Price int64
	// Exclusive: đồ độc quyền, không bán, chỉ ghép từ mảnh.
	Exclusive bool
}

// Kinds là các loại đồ trang trí lắp lên xe.
var Kinds = []string{"spoiler", "neon", "paint", "decal"}

// DefaultPaint là màu sơn của xe mới.
const DefaultPaint = "paint_red"

var Items = []Item{
	{"spoiler_low", "spoiler", "Cánh gió thấp", 10_000, false},
	{"spoiler_mid", "spoiler", "Cánh gió đua", 30_000, false},
	{"spoiler_gt", "spoiler", "Cánh gió GT", 80_000, false},

	{"neon_blue", "neon", "Neon xanh dương", 20_000, false},
	{"neon_pink", "neon", "Neon hồng", 20_000, false},
	{"neon_green", "neon", "Neon xanh lá", 20_000, false},
	{"neon_yellow", "neon", "Neon vàng", 20_000, false},
	{"neon_purple", "neon", "Neon tím", 24_000, false},
	{"neon_red", "neon", "Neon đỏ", 24_000, false},

	{"paint_red", "paint", "Sơn đỏ", 0, false},
	{"paint_blue", "paint", "Sơn xanh dương", 0, false},
	{"paint_yellow", "paint", "Sơn vàng", 0, false},
	{"paint_green", "paint", "Sơn xanh lá", 0, false},
	{"paint_white", "paint", "Sơn trắng", 0, false},
	{"paint_black", "paint", "Sơn đen", 0, false},
	{"paint_orange", "paint", "Sơn cam", 0, false},
	{"paint_purple", "paint", "Sơn tím", 0, false},

	{"decal_number", "decal", "Decal số đua", 10_000, false},
	{"decal_stripe", "decal", "Decal sọc đôi", 16_000, false},
	{"decal_flame", "decal", "Decal lửa", 40_000, false},
	{"decal_lightning", "decal", "Decal tia chớp", 100_000, false},
}

// ExclusiveItems là đồ trang trí độc quyền, chỉ ghép từ mảnh.
var ExclusiveItems = []Item{
	{"paint_x_chroma", "paint", "Sơn ánh kim", 0, true},
	{"decal_x_snow", "decal", "Decal bông tuyết", 0, true},
	{"neon_x_rainbow", "neon", "Neon cầu vồng", 0, true},
	{"spoiler_x_glow", "spoiler", "Cánh gió phát sáng", 0, true},
}

// AllItems là mọi đồ trang trí: đồ bán / sơn miễn phí rồi tới đồ độc quyền.
func AllItems() []Item { return append(append([]Item{}, Items...), ExclusiveItems...) }

func findItem(id string) (Item, bool) {
	for _, it := range AllItems() {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}
