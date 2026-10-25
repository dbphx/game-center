package plaza

import "github.com/dbphx/game-center/internal/chat"

// Clean thay các từ tục trong text bằng *** (bộ lọc dùng chung ở internal/chat).
func Clean(text string) string { return chat.Clean(text) }
