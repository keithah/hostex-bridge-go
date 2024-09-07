package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"maunium.net/go/mautrix/id"
)

type Database struct {
	*sql.DB
	log *zap.Logger
}

func New(path string, log *zap.Logger) (*Database, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	database := &Database{DB: db, log: log}
	err = database.createTables()
	if err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return database, nil
}

func (db *Database) createTables() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS portal (
			hostex_id TEXT PRIMARY KEY,
			matrix_room_id TEXT UNIQUE,
			name TEXT,
			topic TEXT,
			avatar_url TEXT,
			encrypted BOOLEAN,
			last_message_timestamp INTEGER
		);

		CREATE TABLE IF NOT EXISTS message (
			hostex_id TEXT,
			matrix_event_id TEXT UNIQUE,
			timestamp INTEGER,
			sender TEXT,
			content TEXT,
			PRIMARY KEY (hostex_id, matrix_event_id)
		);

		CREATE TABLE IF NOT EXISTS user (
			mxid TEXT PRIMARY KEY,
			hostex_id TEXT UNIQUE
		);
	`)
	return err
}

func (db *Database) StorePortal(hostexID string, roomID id.RoomID, name, topic, avatarURL string, encrypted bool) error {
	_, err := db.Exec(`
		INSERT INTO portal (hostex_id, matrix_room_id, name, topic, avatar_url, encrypted)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (hostex_id) DO UPDATE SET
			matrix_room_id = excluded.matrix_room_id,
			name = excluded.name,
			topic = excluded.topic,
			avatar_url = excluded.avatar_url,
			encrypted = excluded.encrypted
	`, hostexID, roomID, name, topic, avatarURL, encrypted)
	return err
}

func (db *Database) GetPortal(hostexID string) (id.RoomID, error) {
	var roomID id.RoomID
	err := db.QueryRow("SELECT matrix_room_id FROM portal WHERE hostex_id = ?", hostexID).Scan(&roomID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return roomID, err
}

func (db *Database) StoreMessage(hostexID string, eventID id.EventID, timestamp time.Time, sender string, content string) error {
	_, err := db.Exec(`
		INSERT INTO message (hostex_id, matrix_event_id, timestamp, sender, content)
		VALUES (?, ?, ?, ?, ?)
	`, hostexID, eventID, timestamp.Unix(), sender, content)
	return err
}

func (db *Database) GetLastMessageTimestamp(hostexID string) (time.Time, error) {
	var timestamp int64
	err := db.QueryRow("SELECT MAX(timestamp) FROM message WHERE hostex_id = ?", hostexID).Scan(&timestamp)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	return time.Unix(timestamp, 0), err
}

func (db *Database) StoreUser(mxid id.UserID, hostexID string) error {
	_, err := db.Exec(`
		INSERT INTO user (mxid, hostex_id)
		VALUES (?, ?)
		ON CONFLICT (mxid) DO UPDATE SET hostex_id = excluded.hostex_id
	`, mxid, hostexID)
	return err
}

func (db *Database) GetUser(mxid id.UserID) (string, error) {
	var hostexID string
	err := db.QueryRow("SELECT hostex_id FROM user WHERE mxid = ?", mxid).Scan(&hostexID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return hostexID, err
}
