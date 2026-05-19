package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AppUser models a row from app_user. Subject is the OIDC `sub` claim
// and is the natural key the API uses to look up the row associated
// with a request principal.
type AppUser struct {
	UserID      int64
	Subject     string
	Username    string
	DisplayName string
	Email       string
}

// UserRepo is a thin wrapper over pgxpool exposing user-shaped queries.
// It is intentionally narrow: only the operations that handlers need.
type UserRepo struct {
	pool *pgxpool.Pool
}

// NewUserRepo constructs a UserRepo backed by pool.
func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

// Upsert reads or creates the app_user row keyed by sub, refreshes the
// cached username/display/email/claims, and returns the row's
// identity. Designed to be called from per-request middleware as soon
// as a token is verified.
func (r *UserRepo) Upsert(ctx context.Context, sub, username, displayName, email string, claims map[string]any) (AppUser, error) {
	if sub == "" {
		return AppUser{}, errors.New("store: sub must not be empty")
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return AppUser{}, fmt.Errorf("marshal claims: %w", err)
	}

	const q = `
INSERT INTO app_user (sub, username, display_name, email, last_claims)
VALUES ($1, $2, $3, $4, $5::jsonb)
ON CONFLICT (sub) DO UPDATE SET
    username     = EXCLUDED.username,
    display_name = EXCLUDED.display_name,
    email        = EXCLUDED.email,
    last_claims  = EXCLUDED.last_claims
RETURNING user_id, sub, username, display_name, email
`
	var out AppUser
	if err := r.pool.QueryRow(ctx, q, sub, username, displayName, email, claimsJSON).Scan(
		&out.UserID, &out.Subject, &out.Username, &out.DisplayName, &out.Email,
	); err != nil {
		return AppUser{}, fmt.Errorf("upsert user: %w", err)
	}
	return out, nil
}

// GetBySubject fetches the user row keyed by OIDC subject. Returns
// pgx.ErrNoRows when missing — callers should treat that as a 404 or
// pair it with Upsert.
func (r *UserRepo) GetBySubject(ctx context.Context, sub string) (AppUser, error) {
	const q = `
SELECT user_id, sub, username, display_name, email
FROM app_user
WHERE sub = $1
`
	var out AppUser
	if err := r.pool.QueryRow(ctx, q, sub).Scan(
		&out.UserID, &out.Subject, &out.Username, &out.DisplayName, &out.Email,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AppUser{}, ErrNotFound
		}
		return AppUser{}, fmt.Errorf("get user by subject: %w", err)
	}
	return out, nil
}
