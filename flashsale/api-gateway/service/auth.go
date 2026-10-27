// Package service holds the API gateway's business logic, keeping it out of the
// HTTP handlers so it can be exercised without a running server.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/flashsale/api-gateway/repository"
	"github.com/flashsale/common/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials is returned for both an unknown email and a wrong
// password, so the API cannot be used to discover which addresses are
// registered.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrEmailTaken reports a signup for an address that already has an account.
var ErrEmailTaken = repository.ErrEmailTaken

// defaultRole is assigned to every self-registered account. Roles that grant
// more than this are only ever set out of band.
const defaultRole = "user"

// dummyHash is a valid bcrypt hash of a value nobody can supply. Verifying
// against it when an account does not exist makes a failed login take the same
// time whether or not the email is registered, which denies an attacker the
// timing signal that would otherwise enumerate accounts.
var dummyHash []byte

func init() {
	h, err := bcrypt.GenerateFromPassword([]byte(uuid.NewString()), bcrypt.DefaultCost)
	if err != nil {
		panic("auth: cannot generate placeholder hash: " + err.Error())
	}
	dummyHash = h
}

// UserStore is the account storage the auth logic needs.
type UserStore interface {
	Create(ctx context.Context, email, passwordHash, role string) error
	FindCredentialsByEmail(ctx context.Context, email string) (repository.Credentials, error)
}

// CredentialCache caches credential lookups so a login spike does not fall
// through to the database on every request.
type CredentialCache interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, ttl time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// Token is an issued access token and its metadata.
type Token struct {
	Value     string
	ExpiresIn time.Duration
	User      models.User
}

// Auth registers accounts and issues access tokens.
type Auth struct {
	users     UserStore
	cache     CredentialCache
	jwtSecret []byte
	tokenTTL  time.Duration
	cacheTTL  time.Duration
}

// NewAuth creates an Auth service.
func NewAuth(users UserStore, cache CredentialCache, jwtSecret string, tokenTTL, cacheTTL time.Duration) *Auth {
	return &Auth{
		users:     users,
		cache:     cache,
		jwtSecret: []byte(jwtSecret),
		tokenTTL:  tokenTTL,
		cacheTTL:  cacheTTL,
	}
}

// Register creates an account for the email and password.
func (a *Auth) Register(ctx context.Context, email, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := a.users.Create(ctx, email, string(hash), defaultRole); err != nil {
		return err
	}

	// Drop any cached entry for this address so a later login cannot be served
	// from a stale record.
	a.invalidate(ctx, email)
	return nil
}

// Login verifies credentials and issues a signed token.
//
// Credentials are read through a cache-aside path: Redis first, PostgreSQL on a
// miss, then the result is cached. Password verification always happens against
// a hash that never leaves the server.
func (a *Auth) Login(ctx context.Context, email, password string) (Token, error) {
	creds, found := a.lookup(ctx, email)

	// Always run the comparison, even when the account does not exist, so both
	// outcomes cost the same.
	hash := []byte(creds.PasswordHash)
	if !found {
		hash = dummyHash
	}
	if err := bcrypt.CompareHashAndPassword(hash, []byte(password)); err != nil || !found {
		return Token{}, ErrInvalidCredentials
	}

	return a.issueToken(creds.User)
}

// lookup resolves credentials from the cache, falling back to the database.
func (a *Auth) lookup(ctx context.Context, email string) (repository.Credentials, bool) {
	key := cacheKey(email)

	if raw, err := a.cache.Get(ctx, key).Result(); err == nil {
		var cached repository.Credentials
		if err := json.Unmarshal([]byte(raw), &cached); err == nil {
			return cached, true
		}
		// A corrupt entry is not fatal; fall through to the database.
		log.Printf("discarding unreadable cache entry for %s", key)
	} else if !errors.Is(err, redis.Nil) {
		// Redis being unavailable must not break login, but it should be
		// visible rather than silently degrading every request.
		log.Printf("credential cache unavailable: %v", err)
	}

	creds, err := a.users.FindCredentialsByEmail(ctx, email)
	if err != nil {
		if !errors.Is(err, repository.ErrUserNotFound) {
			log.Printf("credential lookup failed: %v", err)
		}
		return repository.Credentials{}, false
	}

	if encoded, err := json.Marshal(creds); err == nil {
		if err := a.cache.Set(ctx, key, encoded, a.cacheTTL).Err(); err != nil {
			log.Printf("failed to cache credentials for %s: %v", key, err)
		}
	}
	return creds, true
}

func (a *Auth) invalidate(ctx context.Context, email string) {
	if err := a.cache.Del(ctx, cacheKey(email)).Err(); err != nil {
		log.Printf("failed to invalidate cached credentials: %v", err)
	}
}

func cacheKey(email string) string { return "user:" + email }

// issueToken signs a token carrying the claims the gateway needs to authorise
// later requests.
func (a *Auth) issueToken(user models.User) (Token, error) {
	expiresAt := time.Now().Add(a.tokenTTL)

	claims := jwt.MapClaims{
		"user_id": user.ID.String(),
		"email":   user.Email,
		"role":    user.Role,
		"exp":     expiresAt.Unix(),
		"iat":     time.Now().Unix(),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.jwtSecret)
	if err != nil {
		return Token{}, fmt.Errorf("sign token: %w", err)
	}

	return Token{Value: signed, ExpiresIn: a.tokenTTL, User: user}, nil
}
