// Package repository contains the API gateway's PostgreSQL data access.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/flashsale/common/models"
	"github.com/lib/pq"
)

// ErrUserNotFound reports that no user matched the lookup.
var ErrUserNotFound = errors.New("user not found")

// ErrEmailTaken reports that an account already exists for the email address.
var ErrEmailTaken = errors.New("email already registered")

// uniqueViolation is the PostgreSQL SQLSTATE for a unique constraint breach.
const uniqueViolation = "23505"

// Credentials is a user together with the stored password hash. The hash is
// deliberately kept out of models.User so it cannot be serialised into an API
// response by accident.
type Credentials struct {
	User         models.User
	PasswordHash string
}

// UserRepository reads and writes user accounts.
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a UserRepository backed by db.
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create stores a new account and returns its ID. It returns ErrEmailTaken when
// the address is already registered, so that a genuine database failure is not
// reported to the caller as a duplicate signup.
func (r *UserRepository) Create(ctx context.Context, email, passwordHash, role string) error {
	const query = `INSERT INTO users (email, password_hash, role) VALUES ($1, $2, $3)`

	_, err := r.db.ExecContext(ctx, query, email, passwordHash, role)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == uniqueViolation {
			return ErrEmailTaken
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// FindCredentialsByEmail loads the account and password hash for an email
// address, returning ErrUserNotFound when there is no such account.
func (r *UserRepository) FindCredentialsByEmail(ctx context.Context, email string) (Credentials, error) {
	const query = `SELECT id, email, password_hash, role FROM users WHERE email = $1`

	var c Credentials
	err := r.db.QueryRowContext(ctx, query, email).
		Scan(&c.User.ID, &c.User.Email, &c.PasswordHash, &c.User.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return Credentials{}, ErrUserNotFound
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("query user by email: %w", err)
	}
	return c, nil
}
