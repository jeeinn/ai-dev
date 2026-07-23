package store

import (
	"fmt"
	"time"
)

// User represents a system user.
type User struct {
	ID                 int64      `json:"id"`
	Username           string     `json:"username"`
	PasswordHash       string     `json:"-"` // Never expose in JSON
	Role               string     `json:"role"`
	DisplayName        string     `json:"display_name"`
	Email              string     `json:"email"`
	IsActive           bool       `json:"is_active"`
	MustChangePassword bool       `json:"must_change_password"`
	LastLogin          *time.Time `json:"last_login"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// CreateUser inserts a new user.
func (db *DB) CreateUser(u *User) error {
	result, err := db.Exec(`INSERT INTO users (username, password_hash, role, display_name, email, is_active, must_change_password)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.Username, u.PasswordHash, u.Role, u.DisplayName, u.Email, u.IsActive, boolToInt(u.MustChangePassword))
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	id, _ := result.LastInsertId()
	u.ID = id
	return nil
}

// GetUser returns a user by ID.
func (db *DB) GetUser(id int64) (*User, error) {
	var u User
	var mustChange int
	err := db.QueryRow(`SELECT id, username, password_hash, role, display_name, email, is_active, must_change_password, last_login, created_at, updated_at
		FROM users WHERE id=?`, id).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.Email, &u.IsActive, &mustChange, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	u.MustChangePassword = mustChange != 0
	return &u, nil
}

// GetUserByUsername returns a user by username.
func (db *DB) GetUserByUsername(username string) (*User, error) {
	var u User
	var mustChange int
	err := db.QueryRow(`SELECT id, username, password_hash, role, display_name, email, is_active, must_change_password, last_login, created_at, updated_at
		FROM users WHERE username=?`, username).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.Email, &u.IsActive, &mustChange, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	u.MustChangePassword = mustChange != 0
	return &u, nil
}

// ListUsers returns all users.
func (db *DB) ListUsers() ([]*User, error) {
	rows, err := db.Query(`SELECT id, username, password_hash, role, display_name, email, is_active, must_change_password, last_login, created_at, updated_at
		FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		var mustChange int
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.Email, &u.IsActive, &mustChange, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u.MustChangePassword = mustChange != 0
		users = append(users, &u)
	}
	return users, nil
}

// UpdateUser updates a user.
func (db *DB) UpdateUser(u *User) error {
	_, err := db.Exec(`UPDATE users SET role=?, display_name=?, email=?, is_active=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`,
		u.Role, u.DisplayName, u.Email, u.IsActive, u.ID)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

// UpdatePassword updates a user's password and clears must_change_password.
func (db *DB) UpdatePassword(id int64, passwordHash string) error {
	_, err := db.Exec(`UPDATE users SET password_hash=?, must_change_password=0, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		passwordHash, id)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// SetMustChangePassword sets or clears the must_change_password flag.
func (db *DB) SetMustChangePassword(id int64, mustChange bool) error {
	_, err := db.Exec(`UPDATE users SET must_change_password=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		boolToInt(mustChange), id)
	if err != nil {
		return fmt.Errorf("set must_change_password: %w", err)
	}
	return nil
}

// UpdateLastLogin updates the last login time.
func (db *DB) UpdateLastLogin(id int64) error {
	_, err := db.Exec(`UPDATE users SET last_login=CURRENT_TIMESTAMP WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	return nil
}

// DeleteUser deletes a user by ID.
func (db *DB) DeleteUser(id int64) error {
	_, err := db.Exec("DELETE FROM users WHERE id=?", id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
