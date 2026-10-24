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
const schemaVersion = 3

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
