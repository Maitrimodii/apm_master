package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
	"github.com/Maitrimodii/apm_master/internal/platform/pg"
)

// Session and password-reset behaviour is decided by SQL (row locks,
// conditional updates, transactions), so these tests run against Postgres.
// They follow the repository convention: skipped unless APM_INTEGRATION_DSN is
// set (see `make test-int`).
const integrationDSNEnv = "APM_INTEGRATION_DSN"

const testPassword = "correct-horse-battery"

type authFixture struct {
	pool     *pgxpool.Pool
	repo     *Repository
	cfg      *config.Config
	sessions *SessionService
	accounts *AccountService
	invites  *InviteService
}

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()

	dsn := os.Getenv(integrationDSNEnv)
	if dsn == "" {
		t.Skipf("set %s to run auth integration tests", integrationDSNEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := pg.Migrate(dsn, "../../migrations"); err != nil {
		t.Fatalf("pg.Migrate() error = %v", err)
	}
	pool, err := pgxpool.New(ctx, strings.Replace(dsn, "pgx5://", "postgres://", 1))
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE TABLE orgs, users CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	cfg := &config.Config{
		App: config.AppConfig{URL: "http://ui.test"},
		Auth: config.AuthConfig{
			JWTSecret:               strings.Repeat("s", 32),
			AccessTokenExpiryMins:   15,
			RefreshTokenExpiryDays:  7,
			RefreshSessionMaxDays:   30,
			InviteExpiryDays:        10,
			PasswordResetExpiryMins: 30,
		},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := NewRepository(pool)
	return &authFixture{
		pool:     pool,
		repo:     repo,
		cfg:      cfg,
		sessions: NewSessionService(repo, cfg, log),
		accounts: NewAccountService(repo, cfg, nil, log),
		invites:  NewInviteService(repo, cfg, nil, log),
	}
}

type testUser struct {
	orgID   uuid.UUID
	orgSlug string
	userID  uuid.UUID
	email   string
}

// createUser inserts an org with one user whose password is testPassword.
func (f *authFixture) createUser(t *testing.T, slug string) testUser {
	t.Helper()
	ctx := context.Background()

	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	passwordHash := string(hash)

	org, err := f.repo.CreateOrg(ctx, f.repo.Query(), Org{Name: slug, Slug: slug})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	email := slug + "@example.com"
	user, err := f.repo.CreateUser(ctx, f.repo.Query(), User{
		OrgID: org.ID, FirstName: "Test", LastName: "User", Email: email, PasswordHash: &passwordHash,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return testUser{orgID: org.ID, orgSlug: slug, userID: user.ID, email: email}
}

func (f *authFixture) login(t *testing.T, u testUser, password string) (*LoginResponse, error) {
	t.Helper()
	return f.sessions.Login(context.Background(), LoginRequest{OrgSlug: u.orgSlug, Email: u.email, Password: password})
}

func (f *authFixture) mustLogin(t *testing.T, u testUser) *LoginResponse {
	t.Helper()
	resp, err := f.login(t, u, testPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return resp
}

func (f *authFixture) refresh(rawToken string) (*LoginResponse, error) {
	return f.sessions.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: rawToken})
}

// tokenRow returns the stored refresh token for a raw token.
func (f *authFixture) tokenRow(t *testing.T, rawToken string) *RefreshToken {
	t.Helper()
	row, err := f.repo.GetRefreshTokenByHash(context.Background(), f.repo.Query(), HashToken(rawToken))
	if err != nil || row == nil {
		t.Fatalf("GetRefreshTokenByHash = %v, %v", row, err)
	}
	return row
}

// countTokens counts refresh tokens matching a WHERE clause over $1.
func (f *authFixture) countTokens(t *testing.T, where string, arg any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM refresh_tokens WHERE "+where, arg).Scan(&n); err != nil {
		t.Fatalf("count refresh tokens: %v", err)
	}
	return n
}

func (f *authFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// requireAppErr fails unless err is an *apperr.Error with the given code.
func requireAppErr(t *testing.T, err error, want apperr.Code) {
	t.Helper()
	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %v, want *apperr.Error with code %s", err, want)
	}
	if appErr.Code != want {
		t.Fatalf("error code = %s (%s), want %s", appErr.Code, appErr.Message, want)
	}
}

// runConcurrently starts n calls of fn at the same moment and returns their
// errors.
func runConcurrently(n int, fn func(i int) error) []error {
	start := make(chan struct{})
	errs := make([]error, n)
	done := make(chan struct{})
	for i := range n {
		go func() {
			defer func() { done <- struct{}{} }()
			<-start
			errs[i] = fn(i)
		}()
	}
	close(start)
	for range n {
		<-done
	}
	return errs
}

// useInviteToken makes the next invite tokens equal to raw. The API never
// returns invite tokens (only the email carries them), so tests that accept
// an invite fix the token instead.
func (f *authFixture) useInviteToken(raw string) string {
	f.invites.newToken = func() (string, error) { return raw, nil }
	return raw
}

// createAdmin registers an org the way production does, so the admin role is
// seeded with the full permission catalogue.
func (f *authFixture) createAdmin(t *testing.T, slug string) testUser {
	t.Helper()
	email := "admin@" + slug + ".example.com"
	resp, err := f.accounts.RegisterOrgOwner(context.Background(), RegisterOrgOwnerRequest{
		OrgName: slug, OrgSlug: slug, FirstName: "Org", LastName: "Admin", Email: email, Password: testPassword,
	})
	if err != nil {
		t.Fatalf("RegisterOrgOwner: %v", err)
	}
	return testUser{orgID: resp.Org.ID, orgSlug: slug, userID: resp.User.ID, email: email}
}

// permissionID returns the ID of a catalogue permission such as "traces:read".
func (f *authFixture) permissionID(t *testing.T, permission string) uuid.UUID {
	t.Helper()
	name, action, _ := strings.Cut(permission, ":")
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		"SELECT id FROM permissions WHERE name = $1 AND action = $2", name, action).Scan(&id); err != nil {
		t.Fatalf("permission %s: %v", permission, err)
	}
	return id
}

func (f *authFixture) permissionIDs(t *testing.T, permissions ...string) []uuid.UUID {
	t.Helper()
	ids := make([]uuid.UUID, 0, len(permissions))
	for _, p := range permissions {
		ids = append(ids, f.permissionID(t, p))
	}
	return ids
}

// createRole inserts a custom role with the given permissions directly,
// bypassing the grant policy (test setup, not the behaviour under test).
func (f *authFixture) createRole(t *testing.T, orgID uuid.UUID, name string, permissions ...string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	role, err := f.repo.CreateRole(ctx, f.repo.Query(), Role{OrgID: orgID, Name: name})
	if err != nil {
		t.Fatalf("CreateRole %s: %v", name, err)
	}
	for _, id := range f.permissionIDs(t, permissions...) {
		if err := f.repo.AssignPermissionToRole(ctx, f.repo.Query(), role.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	return role.ID
}

func (f *authFixture) adminRoleID(t *testing.T, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		"SELECT id FROM roles WHERE org_id = $1 AND name = 'admin' AND is_system", orgID).Scan(&id); err != nil {
		t.Fatalf("admin role: %v", err)
	}
	return id
}

// createMember adds a user with password testPassword and the given roles to
// admin's org.
func (f *authFixture) createMember(t *testing.T, admin testUser, name string, roleIDs ...uuid.UUID) testUser {
	t.Helper()
	ctx := context.Background()
	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	passwordHash := string(hash)
	email := name + "@" + admin.orgSlug + ".example.com"
	user, err := f.repo.CreateUser(ctx, f.repo.Query(), User{
		OrgID: admin.orgID, FirstName: name, LastName: "Member", Email: email, PasswordHash: &passwordHash,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	for _, roleID := range roleIDs {
		if err := f.repo.AssignRoleToUser(ctx, f.repo.Query(), user.ID, roleID); err != nil {
			t.Fatal(err)
		}
	}
	return testUser{orgID: admin.orgID, orgSlug: admin.orgSlug, userID: user.ID, email: email}
}

// rolePermissionNames returns a role's permissions as sorted "name:action".
func (f *authFixture) rolePermissionNames(t *testing.T, roleID uuid.UUID) []string {
	t.Helper()
	rows, err := f.pool.Query(context.Background(), `
		SELECT p.name || ':' || p.action FROM role_permissions rp
		JOIN permissions p ON p.id = rp.permission_id
		WHERE rp.role_id = $1 ORDER BY 1`, roleID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	return names
}
