package plaza

import (
	"regexp"
	"strings"
)

// Lọc vài từ tục phổ biến trong chat: so theo từng từ (không theo chuỗi con) để không che nhầm
// từ thường như "lon bia", "dâm bụt"; từ bị lọc thay bằng ***.
var banned = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`địt đụ đjt djt lồn cặc buồi đéo đĩ
		đm dm đmm dmm đcm dcm cđm cdm đkm dkm vcl vkl vcc clm cmm loz
		fuck fucking shit bitch`) {
		banned[w] = true
	}
}

var wordRe = regexp.MustCompile(`[\p{L}\p{N}]+`)

// Clean thay các từ tục trong text bằng ***.
func Clean(text string) string {
	return wordRe.ReplaceAllStringFunc(text, func(w string) string {
		if banned[strings.ToLower(w)] {
			return "***"
		}
		return w
	})
}
