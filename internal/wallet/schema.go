package wallet

import (
	"context"
	"database/sql"
	"fmt"
)

// schemaVersion lưu trong PRAGMA user_version.
//
//	1: users giữ token tài khoản khách ngay trong bảng.
//	2: thêm username / mật khẩu, phiên đăng nhập tách sang bảng sessions.
//	3: thêm nhân vật (giới tính, trang phục) và điểm kinh nghiệm.
//	4: trang phục phải mua ở shop; bảng user_outfits lưu các món đã có.
//	5: máu (HP), nằm viện, thời điểm va chạm xe gần nhất.
//	6: tóc và cánh mua ở shop thời trang; bảng user_items lưu các món đã có.
//	7: gương mặt mua ở shop thời trang (lưu chung bảng user_items, kind 'face').
const schemaVersion = 7

const schemaV2 = `
CREATE TABLE users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT UNIQUE COLLATE NOCASE, -- NULL với tài khoản khách cũ chưa đặt tên đăng nhập
	password_hash TEXT NOT NULL DEFAULT '',
	name          TEXT NOT NULL,
	balance       INTEGER NOT NULL CHECK (balance >= 0),
	last_daily    TEXT NOT NULL DEFAULT '',
	created_at    INTEGER NOT NULL
);
CREATE TABLE sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id),
	created_at INTEGER NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);
CREATE TABLE ledger (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id       INTEGER NOT NULL REFERENCES users(id),
	delta         INTEGER NOT NULL,
	balance_after INTEGER NOT NULL,
	game          TEXT NOT NULL,
	note          TEXT NOT NULL,
	created_at    INTEGER NOT NULL
);
CREATE INDEX ledger_user ON ledger(user_id, id);
`

// Từ v1: chuyển token khách sang sessions (người đang chơi không bị đăng xuất), dựng lại bảng users.
const migrateV1toV2 = `
CREATE TABLE sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id),
	created_at INTEGER NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);
INSERT INTO sessions (token_hash, user_id, created_at) SELECT token_hash, id, created_at FROM users;
CREATE TABLE users_v2 (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT UNIQUE COLLATE NOCASE,
	password_hash TEXT NOT NULL DEFAULT '',
	name          TEXT NOT NULL,
	balance       INTEGER NOT NULL CHECK (balance >= 0),
	last_daily    TEXT NOT NULL DEFAULT '',
	created_at    INTEGER NOT NULL
);
INSERT INTO users_v2 (id, name, balance, last_daily, created_at) SELECT id, name, balance, last_daily, created_at FROM users;
DROP TABLE users;
ALTER TABLE users_v2 RENAME TO users;
`

const migrateV2toV3 = `
ALTER TABLE users ADD COLUMN gender TEXT NOT NULL DEFAULT 'male';
ALTER TABLE users ADD COLUMN outfit TEXT NOT NULL DEFAULT 'casual';
ALTER TABLE users ADD COLUMN xp INTEGER NOT NULL DEFAULT 0 CHECK (xp >= 0);
`

// Người chơi cũ giữ trang phục đang mặc (trước đây được chọn miễn phí).
const migrateV3toV4 = `
CREATE TABLE user_outfits (
	user_id    INTEGER NOT NULL REFERENCES users(id),
	outfit     TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, outfit)
);
INSERT INTO user_outfits (user_id, outfit, created_at) SELECT id, outfit, created_at FROM users;
`

// Người chơi cũ được đầy máu (hp_at NULL nghĩa là chưa tính lần nào, coi như đầy).
const migrateV4toV5 = `
ALTER TABLE users ADD COLUMN hp INTEGER NOT NULL DEFAULT 100;
ALTER TABLE users ADD COLUMN hp_at INTEGER;
ALTER TABLE users ADD COLUMN hospital_until INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN last_hit_ms INTEGER NOT NULL DEFAULT 0;
`

// Người chơi cũ để tóc mặc định, không cánh.
const migrateV5toV6 = `
ALTER TABLE users ADD COLUMN hair TEXT NOT NULL DEFAULT 'default';
ALTER TABLE users ADD COLUMN wings TEXT NOT NULL DEFAULT '';
CREATE TABLE user_items (
	user_id    INTEGER NOT NULL REFERENCES users(id),
	kind       TEXT NOT NULL, -- 'hair', 'wings' hoặc 'face'
	item       TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, kind, item)
);
`

// Người chơi cũ để gương mặt mặc định.
const migrateV6toV7 = `
ALTER TABLE users ADD COLUMN face TEXT NOT NULL DEFAULT 'default';
`

func migrate(db *sql.DB) error {
	ctx := context.Background()
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version == schemaVersion {
		return nil
	}
	var hasUsers bool
	if err := db.QueryRowContext(ctx, `SELECT count(*) > 0 FROM sqlite_master WHERE type = 'table' AND name = 'users'`).Scan(&hasUsers); err != nil {
		return err
	}

	if version > schemaVersion {
		return fmt.Errorf("database phiên bản %d mới hơn chương trình (%d)", version, schemaVersion)
	}
	// Database mới tạo thẳng bản v2 rồi chạy tiếp các bước như database cũ.
	var steps []string
	if !hasUsers {
		steps, version = append(steps, schemaV2), 2
	}
	if version <= 1 {
		steps = append(steps, migrateV1toV2)
	}
	if version <= 2 {
		steps = append(steps, migrateV2toV3)
	}
	if version <= 3 {
		steps = append(steps, migrateV3toV4)
	}
	if version <= 4 {
		steps = append(steps, migrateV4toV5)
	}
	if version <= 5 {
		steps = append(steps, migrateV5toV6)
	}
	if version <= 6 {
		steps = append(steps, migrateV6toV7)
	}

	// Dựng lại bảng có khoá ngoại trỏ tới cần tắt kiểm tra khoá ngoại (không đổi được trong transaction).
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer db.ExecContext(ctx, `PRAGMA foreign_keys = ON`)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, script := range steps {
		if _, err := tx.ExecContext(ctx, script); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}
