package db

import (
	"database/sql"
	"time"

	"soft-proxy/internal/logger"

	_ "modernc.org/sqlite"
)

type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Protocol  string    `json:"protocol"`
	UUID      string    `json:"uuid"`
	Status    string    `json:"status"`
	Traffic   string    `json:"traffic"`
	CreatedAt time.Time `json:"created_at"`
}

var dbConn *sql.DB

func InitDB(dbPath string) error {
	var err error
	dbConn, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	if err = dbConn.Ping(); err != nil {
		dbConn.Close()
		return err
	}

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		protocol TEXT NOT NULL,
		uuid TEXT NOT NULL,
		status TEXT NOT NULL,
		traffic TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err = dbConn.Exec(createTableQuery)
	if err != nil {
		dbConn.Close()
		return err
	}

	logger.Info("Database initialized at %s", dbPath)
	return nil
}

func GetUsers() ([]User, error) {
	rows, err := dbConn.Query("SELECT id, name, protocol, uuid, status, traffic, created_at FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Protocol, &u.UUID, &u.Status, &u.Traffic, &u.CreatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func AddUser(name, protocol, uuid string) error {
	_, err := dbConn.Exec("INSERT INTO users (name, protocol, uuid, status, traffic) VALUES (?, ?, ?, 'active', '0 GB')", name, protocol, uuid)
	return err
}

func DeleteUser(id int) error {
	_, err := dbConn.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func CloseDB() error {
	if dbConn != nil {
		return dbConn.Close()
	}
	return nil
}
