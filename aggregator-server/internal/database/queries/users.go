package queries

import (
	"time"

	"github.com/Fimeg/RedFlag/aggregator-server/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

type UserQueries struct {
	db *sqlx.DB
}

func NewUserQueries(db *sqlx.DB) *UserQueries {
	return &UserQueries{db: db}
}

// CreateUser inserts a new user into the database with password hashing
func (q *UserQueries) CreateUser(username, email, password, role string) (*models.User, error) {
	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		ID:           uuid.New(),
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}

	query := `
		INSERT INTO users (
			id, username, email, password_hash, role, created_at
		) VALUES (
			:id, :username, :email, :password_hash, :role, :created_at
		)
		RETURNING *
	`

	rows, err := q.db.NamedQuery(query, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		if err := rows.StructScan(user); err != nil {
			return nil, err
		}
		return user, nil
	}

	return nil, nil
}

// GetUserByUsername retrieves a user by username
func (q *UserQueries) GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	query := `SELECT * FROM users WHERE username = $1`
	err := q.db.Get(&user, query, username)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// VerifyCredentials checks if the provided username and password are valid
func (q *UserQueries) VerifyCredentials(username, password string) (*models.User, error) {
	user, err := q.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}

	// Compare the provided password with the stored hash
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, err // Invalid password
	}

	// Update last login time
	q.UpdateLastLogin(user.ID)

	// Don't return password hash
	user.PasswordHash = ""
	return user, nil
}

// UpdateLastLogin updates the user's last login timestamp
func (q *UserQueries) UpdateLastLogin(id uuid.UUID) error {
	query := `UPDATE users SET last_login = $1 WHERE id = $2`
	_, err := q.db.Exec(query, time.Now().UTC(), id)
	return err
}

// GetUserByID retrieves a user by ID
func (q *UserQueries) GetUserByID(id uuid.UUID) (*models.User, error) {
	var user models.User
	query := `SELECT id, username, email, role, created_at, last_login FROM users WHERE id = $1`
	err := q.db.Get(&user, query, id)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// EnsureAdminUser creates an admin user if one doesn't exist
func (q *UserQueries) EnsureAdminUser(username, email, password string) error {
	// Check if admin user already exists
	existingUser, err := q.GetUserByUsername(username)
	if err == nil && existingUser != nil {
		return nil // Admin user already exists
	}

	// Create admin user
	_, err = q.CreateUser(username, email, password, "admin")
	return err
}