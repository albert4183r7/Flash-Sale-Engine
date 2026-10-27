package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/flashsale/api-gateway/repository"
	"github.com/flashsale/api-gateway/service"
	"github.com/flashsale/common/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const (
	testSecret   = "a-test-secret-at-least-16-chars"
	testPassword = "correct-horse-battery"
)

// fakeUsers is an in-memory UserStore.
type fakeUsers struct {
	byEmail map[string]repository.Credentials
	// findErr, when set, is returned instead of a lookup result.
	findErr error
	// createErr, when set, is returned instead of inserting.
	createErr error
	finds     int
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byEmail: map[string]repository.Credentials{}}
}

func (f *fakeUsers) add(t *testing.T, email, password, role string) repository.Credentials {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	creds := repository.Credentials{
		User:         models.User{ID: uuid.New(), Email: email, Role: role},
		PasswordHash: string(hash),
	}
	f.byEmail[email] = creds
	return creds
}

func (f *fakeUsers) Create(_ context.Context, email, passwordHash, role string) error {
	if f.createErr != nil {
		return f.createErr
	}
	if _, exists := f.byEmail[email]; exists {
		return repository.ErrEmailTaken
	}
	f.byEmail[email] = repository.Credentials{
		User:         models.User{ID: uuid.New(), Email: email, Role: role},
		PasswordHash: passwordHash,
	}
	return nil
}

func (f *fakeUsers) FindCredentialsByEmail(_ context.Context, email string) (repository.Credentials, error) {
	f.finds++
	if f.findErr != nil {
		return repository.Credentials{}, f.findErr
	}
	creds, ok := f.byEmail[email]
	if !ok {
		return repository.Credentials{}, repository.ErrUserNotFound
	}
	return creds, nil
}

// fakeCache is an in-memory CredentialCache.
type fakeCache struct {
	mu      sync.Mutex
	entries map[string]string
	// down simulates Redis being unreachable.
	down bool
	sets int
	dels int
}

func newFakeCache() *fakeCache { return &fakeCache{entries: map[string]string{}} }

func (f *fakeCache) Get(_ context.Context, key string) *redis.StringCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return redis.NewStringResult("", errors.New("connection refused"))
	}
	value, ok := f.entries[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(value, nil)
}

func (f *fakeCache) Set(_ context.Context, key string, value any, _ time.Duration) *redis.StatusCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return redis.NewStatusResult("", errors.New("connection refused"))
	}
	f.sets++
	switch v := value.(type) {
	case []byte:
		f.entries[key] = string(v)
	case string:
		f.entries[key] = v
	}
	return redis.NewStatusResult("OK", nil)
}

func (f *fakeCache) Del(_ context.Context, keys ...string) *redis.IntCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return redis.NewIntResult(0, errors.New("connection refused"))
	}
	f.dels++
	for _, k := range keys {
		delete(f.entries, k)
	}
	return redis.NewIntResult(int64(len(keys)), nil)
}

func newAuth(users service.UserStore, cache service.CredentialCache) *service.Auth {
	return service.NewAuth(users, cache, testSecret, time.Hour, time.Hour)
}

func TestLoginIssuesUsableToken(t *testing.T) {
	users := newFakeUsers()
	creds := users.add(t, "buyer@example.com", testPassword, "user")
	auth := newAuth(users, newFakeCache())

	token, err := auth.Login(context.Background(), "buyer@example.com", testPassword)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if token.User.ID != creds.User.ID {
		t.Errorf("token user id = %s, want %s", token.User.ID, creds.User.ID)
	}
	if token.ExpiresIn != time.Hour {
		t.Errorf("ExpiresIn = %s, want 1h0m0s", token.ExpiresIn)
	}

	// The token must actually verify against the configured secret and carry
	// the claims the gateway authorises on.
	var claims jwt.MapClaims
	parsed, err := jwt.ParseWithClaims(token.Value, &claims,
		func(*jwt.Token) (any, error) { return []byte(testSecret), nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !parsed.Valid {
		t.Fatalf("issued token does not verify: %v", err)
	}
	if claims["user_id"] != creds.User.ID.String() {
		t.Errorf("user_id claim = %v, want %s", claims["user_id"], creds.User.ID)
	}
	if claims["role"] != "user" {
		t.Errorf("role claim = %v, want %q", claims["role"], "user")
	}
}

// A wrong password and an unknown account must be indistinguishable, otherwise
// the endpoint tells an attacker which email addresses are registered.
func TestLoginRejectsBadCredentials(t *testing.T) {
	users := newFakeUsers()
	users.add(t, "buyer@example.com", testPassword, "user")
	auth := newAuth(users, newFakeCache())

	tests := []struct {
		name     string
		email    string
		password string
	}{
		{"wrong password", "buyer@example.com", "wrong-password"},
		{"unknown account", "nobody@example.com", testPassword},
		{"empty password", "buyer@example.com", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.Login(context.Background(), tt.email, tt.password)
			if !errors.Is(err, service.ErrInvalidCredentials) {
				t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
			}
		})
	}
}

// The second login must be served from the cache, which is the whole point of
// the cache-aside path during a login spike.
func TestLoginCachesCredentials(t *testing.T) {
	users := newFakeUsers()
	users.add(t, "buyer@example.com", testPassword, "user")
	cache := newFakeCache()
	auth := newAuth(users, cache)

	for i := 0; i < 3; i++ {
		if _, err := auth.Login(context.Background(), "buyer@example.com", testPassword); err != nil {
			t.Fatalf("Login() attempt %d error = %v", i+1, err)
		}
	}

	if users.finds != 1 {
		t.Errorf("database was queried %d times, want 1: later logins should hit the cache", users.finds)
	}
	if cache.sets != 1 {
		t.Errorf("cache was written %d times, want 1", cache.sets)
	}
}

// Losing Redis must degrade to database lookups, not break logins outright.
func TestLoginSurvivesCacheOutage(t *testing.T) {
	users := newFakeUsers()
	users.add(t, "buyer@example.com", testPassword, "user")
	cache := newFakeCache()
	cache.down = true
	auth := newAuth(users, cache)

	if _, err := auth.Login(context.Background(), "buyer@example.com", testPassword); err != nil {
		t.Fatalf("Login() with an unavailable cache error = %v, want success", err)
	}
}

// A corrupt cache entry must not lock a user out; the database is the fallback.
func TestLoginRecoversFromCorruptCacheEntry(t *testing.T) {
	users := newFakeUsers()
	users.add(t, "buyer@example.com", testPassword, "user")
	cache := newFakeCache()
	cache.entries["user:buyer@example.com"] = "{not json"
	auth := newAuth(users, cache)

	if _, err := auth.Login(context.Background(), "buyer@example.com", testPassword); err != nil {
		t.Fatalf("Login() with a corrupt cache entry error = %v, want success", err)
	}
}

// A database outage must not be reported as bad credentials in a way that
// succeeds; it must fail the login.
func TestLoginFailsWhenStoreIsDown(t *testing.T) {
	users := newFakeUsers()
	users.findErr = errors.New("connection refused")
	auth := newAuth(users, newFakeCache())

	if _, err := auth.Login(context.Background(), "buyer@example.com", testPassword); err == nil {
		t.Fatal("Login() with an unavailable database = nil, want an error")
	}
}

func TestRegisterCreatesLoginableAccount(t *testing.T) {
	users := newFakeUsers()
	auth := newAuth(users, newFakeCache())

	if err := auth.Register(context.Background(), "new@example.com", testPassword); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// The password must be stored hashed, never in the clear.
	stored := users.byEmail["new@example.com"]
	if stored.PasswordHash == testPassword {
		t.Fatal("password was stored in plain text")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(testPassword)); err != nil {
		t.Fatalf("stored hash does not match the password: %v", err)
	}

	if _, err := auth.Login(context.Background(), "new@example.com", testPassword); err != nil {
		t.Fatalf("Login() after Register() error = %v", err)
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	users := newFakeUsers()
	users.add(t, "taken@example.com", testPassword, "user")
	auth := newAuth(users, newFakeCache())

	err := auth.Register(context.Background(), "taken@example.com", testPassword)
	if !errors.Is(err, service.ErrEmailTaken) {
		t.Fatalf("Register() error = %v, want ErrEmailTaken", err)
	}
}

// A genuine database failure must not be reported as a duplicate signup, which
// would tell the caller an account exists when it does not.
func TestRegisterReportsStoreFailure(t *testing.T) {
	users := newFakeUsers()
	users.createErr = errors.New("connection refused")
	auth := newAuth(users, newFakeCache())

	err := auth.Register(context.Background(), "new@example.com", testPassword)
	if err == nil {
		t.Fatal("Register() = nil, want an error")
	}
	if errors.Is(err, service.ErrEmailTaken) {
		t.Fatal("a database outage was reported as a duplicate email")
	}
}

// Registering must clear any cached entry for the address, so a stale record
// cannot serve a later login.
func TestRegisterInvalidatesCache(t *testing.T) {
	users := newFakeUsers()
	cache := newFakeCache()
	cache.entries["user:new@example.com"] = "stale"
	auth := newAuth(users, cache)

	if err := auth.Register(context.Background(), "new@example.com", testPassword); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, exists := cache.entries["user:new@example.com"]; exists {
		t.Error("cached entry survived registration")
	}
}

// Whatever is cached must round-trip: a hash that cannot be read back would
// silently push every login to the database.
func TestCachedCredentialsRoundTrip(t *testing.T) {
	users := newFakeUsers()
	users.add(t, "buyer@example.com", testPassword, "user")
	cache := newFakeCache()
	auth := newAuth(users, cache)

	if _, err := auth.Login(context.Background(), "buyer@example.com", testPassword); err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	var cached repository.Credentials
	if err := json.Unmarshal([]byte(cache.entries["user:buyer@example.com"]), &cached); err != nil {
		t.Fatalf("cached entry is not valid JSON: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(cached.PasswordHash), []byte(testPassword)); err != nil {
		t.Fatalf("cached hash does not verify the password: %v", err)
	}
}
