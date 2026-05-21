package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// User is the admin-side projection of an app_user row used by the
// /users CRUD endpoints. It carries the privilege + status columns
// added in migration 0005, plus optional joined lists of
// collection-grant memberships and user_group memberships. Sentinel
// nulls preserve the OpenAPI "null when unset" contract for
// StatusDate / StatusUserID / Sub.
type User struct {
	UserID                    int64
	Sub                       *string
	Username                  string
	DisplayName               string
	Email                     string
	Status                    string // "available" | "unavailable"
	StatusDate                *time.Time
	StatusUserID              *int64
	PrivilegeAdmin            bool
	PrivilegeCreateCollection bool
	LastAccessUnix            *int64
	UserGroups                []UserGroupBasic
	CollectionGrants          []UserCollectionGrant
}

// UserCollectionGrant is the per-user view of a single collection_grant
// row scoped to the user (rather than a user_group).
type UserCollectionGrant struct {
	GrantID        int64
	CollectionID   int64
	CollectionName string
	RoleID         int16
}

// UserGroupBasic is the user_group projection embedded in User
// responses (id + name + nil role_id, since a group doesn't have a
// role outside of a collection grant context).
type UserGroupBasic struct {
	UserGroupID int64
	Name        string
}

// UserCreate is the input bundle for UserRepo.CreateAdmin (mirrors the
// UserPost OpenAPI shape).
type UserCreate struct {
	Username                  string
	Status                    string
	PrivilegeAdmin            bool
	PrivilegeCreateCollection bool
	UserGroups                []int64
	CollectionGrants          []GrantCreate
}

// UserPatch is the input bundle for UserRepo.PatchAdmin. Each non-nil
// field is applied; nil fields are left untouched. CollectionGrants /
// UserGroups, when non-nil, fully replace the existing lists (matching
// upstream PUT semantics within a PATCH for these collections).
type UserPatch struct {
	Username                  *string
	Status                    *string
	PrivilegeAdmin            *bool
	PrivilegeCreateCollection *bool
	UserGroups                *[]int64
	CollectionGrants          *[]GrantCreate
}

// UserListFilter narrows ListAdmin.
type UserListFilter struct {
	Username      string
	UsernameMatch string // "exact" | "contains" | "startsWith" | "endsWith"
	Status        string // "available" | "unavailable" | ""
	Privilege     string // "admin" | "create_collection" | ""
}

// ListAdmin returns all users that match filter, ordered by username.
// The returned rows are *not* projected with collection grants or
// user_groups (those are populated by Get for the single-user view).
func (r *UserRepo) ListAdmin(ctx context.Context, filter UserListFilter) ([]User, error) {
	q := `
SELECT user_id, sub, username, display_name, email,
       status, status_date, status_user_id,
       privilege_admin, privilege_create_collection, last_access_unix
FROM app_user
WHERE 1=1`
	args := []any{}
	idx := 1

	if filter.Username != "" {
		// usernameMatch chooses LIKE shape (or = for exact).
		switch strings.ToLower(filter.UsernameMatch) {
		case "exact", "":
			q += fmt.Sprintf(" AND lower(username) = lower($%d)", idx)
			args = append(args, filter.Username)
		case "startswith":
			q += fmt.Sprintf(" AND lower(username) LIKE lower($%d)", idx)
			args = append(args, filter.Username+"%")
		case "endswith":
			q += fmt.Sprintf(" AND lower(username) LIKE lower($%d)", idx)
			args = append(args, "%"+filter.Username)
		case "contains":
			q += fmt.Sprintf(" AND lower(username) LIKE lower($%d)", idx)
			args = append(args, "%"+filter.Username+"%")
		default:
			return nil, fmt.Errorf("store: invalid usernameMatch %q", filter.UsernameMatch)
		}
		idx++
	}
	if filter.Status != "" {
		q += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, filter.Status)
		idx++
	}
	switch filter.Privilege {
	case "admin":
		q += " AND privilege_admin = true"
	case "create_collection":
		q += " AND privilege_create_collection = true"
	case "":
	default:
		return nil, fmt.Errorf("store: invalid privilege %q", filter.Privilege)
	}
	q += " ORDER BY lower(username) ASC"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GetAdmin returns a single user with collection grants and user
// groups joined.
func (r *UserRepo) GetAdmin(ctx context.Context, userID int64) (User, error) {
	const q = `
SELECT user_id, sub, username, display_name, email,
       status, status_date, status_user_id,
       privilege_admin, privilege_create_collection, last_access_unix
FROM app_user
WHERE user_id = $1
`
	row := r.pool.QueryRow(ctx, q, userID)
	u, err := scanUserRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	if err := r.attachUserExtras(ctx, &u); err != nil {
		return User{}, err
	}
	return u, nil
}

// CreateAdmin inserts a new app_user (no OIDC sub yet) plus optional
// user_group / collection_grant rows in a single transaction.
func (r *UserRepo) CreateAdmin(ctx context.Context, in UserCreate) (User, error) {
	if in.Username == "" {
		return User{}, errors.New("store: username required")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return User{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	status := in.Status
	if status == "" {
		status = "available"
	}

	const insQ = `
INSERT INTO app_user (sub, username, display_name, email, status,
                      privilege_admin, privilege_create_collection)
VALUES (NULL, $1, '', '', $2, $3, $4)
RETURNING user_id
`
	var uid int64
	if err := tx.QueryRow(ctx, insQ, in.Username, status,
		in.PrivilegeAdmin, in.PrivilegeCreateCollection,
	).Scan(&uid); err != nil {
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	if err := setUserGroupsTx(ctx, tx, uid, in.UserGroups); err != nil {
		return User{}, err
	}
	if err := setCollectionGrantsTx(ctx, tx, uid, in.CollectionGrants); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit tx: %w", err)
	}
	return r.GetAdmin(ctx, uid)
}

// PatchAdmin applies a partial update.
func (r *UserRepo) PatchAdmin(ctx context.Context, userID int64, p UserPatch) (User, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return User{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	setClauses := []string{}
	args := []any{}
	idx := 1
	if p.Username != nil {
		setClauses = append(setClauses, fmt.Sprintf("username = $%d", idx))
		args = append(args, *p.Username)
		idx++
	}
	if p.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d, status_date = now()", idx))
		args = append(args, *p.Status)
		idx++
	}
	if p.PrivilegeAdmin != nil {
		setClauses = append(setClauses, fmt.Sprintf("privilege_admin = $%d", idx))
		args = append(args, *p.PrivilegeAdmin)
		idx++
	}
	if p.PrivilegeCreateCollection != nil {
		setClauses = append(setClauses, fmt.Sprintf("privilege_create_collection = $%d", idx))
		args = append(args, *p.PrivilegeCreateCollection)
		idx++
	}
	if len(setClauses) > 0 {
		q := "UPDATE app_user SET " + strings.Join(setClauses, ", ") +
			fmt.Sprintf(" WHERE user_id = $%d", idx)
		args = append(args, userID)
		ct, err := tx.Exec(ctx, q, args...)
		if err != nil {
			return User{}, fmt.Errorf("update user: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return User{}, ErrNotFound
		}
	} else {
		// Confirm the user exists even when no scalar fields change.
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT true FROM app_user WHERE user_id = $1`, userID).Scan(&exists); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return User{}, ErrNotFound
			}
			return User{}, err
		}
	}
	if p.UserGroups != nil {
		if err := setUserGroupsTx(ctx, tx, userID, *p.UserGroups); err != nil {
			return User{}, err
		}
	}
	if p.CollectionGrants != nil {
		if err := setCollectionGrantsTx(ctx, tx, userID, *p.CollectionGrants); err != nil {
			return User{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit tx: %w", err)
	}
	return r.GetAdmin(ctx, userID)
}

// DeleteAdmin removes a user that has never accessed the system
// (sub IS NULL). Returns ErrConflict if last_access_unix is non-null
// or sub is bound.
func (r *UserRepo) DeleteAdmin(ctx context.Context, userID int64) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var sub *string
	var lastAccess *int64
	if err := tx.QueryRow(ctx,
		`SELECT sub, last_access_unix FROM app_user WHERE user_id = $1`, userID,
	).Scan(&sub, &lastAccess); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if sub != nil || lastAccess != nil {
		return ErrConflict
	}
	if _, err := tx.Exec(ctx, `DELETE FROM app_user WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return tx.Commit(ctx)
}

// AttachExtras fills in CollectionGrants + UserGroups on u. Public so
// handlers can hydrate the projections lazily for list views.
func (r *UserRepo) AttachExtras(ctx context.Context, u *User) error {
	return r.attachUserExtras(ctx, u)
}

// attachUserExtras fills in CollectionGrants + UserGroups on u.
func (r *UserRepo) attachUserExtras(ctx context.Context, u *User) error {
	grants, err := r.userGrants(ctx, u.UserID)
	if err != nil {
		return err
	}
	u.CollectionGrants = grants
	groups, err := r.userGroups(ctx, u.UserID)
	if err != nil {
		return err
	}
	u.UserGroups = groups
	return nil
}

func (r *UserRepo) userGrants(ctx context.Context, userID int64) ([]UserCollectionGrant, error) {
	const q = `
SELECT cg.grant_id, cg.collection_id, c.name, cg.role_id
FROM collection_grant cg
JOIN collection c ON c.collection_id = cg.collection_id
WHERE cg.user_id = $1
ORDER BY lower(c.name) ASC
`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("user grants: %w", err)
	}
	defer rows.Close()
	var out []UserCollectionGrant
	for rows.Next() {
		var g UserCollectionGrant
		if err := rows.Scan(&g.GrantID, &g.CollectionID, &g.CollectionName, &g.RoleID); err != nil {
			return nil, fmt.Errorf("scan user grant: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *UserRepo) userGroups(ctx context.Context, userID int64) ([]UserGroupBasic, error) {
	const q = `
SELECT ug.user_group_id, ug.name
FROM user_group_user ugu
JOIN user_group ug ON ug.user_group_id = ugu.user_group_id
WHERE ugu.user_id = $1
ORDER BY lower(ug.name) ASC
`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("user groups: %w", err)
	}
	defer rows.Close()
	var out []UserGroupBasic
	for rows.Next() {
		var g UserGroupBasic
		if err := rows.Scan(&g.UserGroupID, &g.Name); err != nil {
			return nil, fmt.Errorf("scan user group: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// pgRow is the narrow surface scanUserRow needs from both pgx.Rows and
// pgx.Row so list and get can share the column projection.
type pgRow interface {
	Scan(dest ...any) error
}

func scanUserRow(row pgRow) (User, error) {
	var u User
	if err := row.Scan(
		&u.UserID, &u.Sub, &u.Username, &u.DisplayName, &u.Email,
		&u.Status, &u.StatusDate, &u.StatusUserID,
		&u.PrivilegeAdmin, &u.PrivilegeCreateCollection, &u.LastAccessUnix,
	); err != nil {
		return User{}, err
	}
	return u, nil
}

// setUserGroupsTx replaces u's user_group membership with groupIDs.
func setUserGroupsTx(ctx context.Context, tx pgx.Tx, userID int64, groupIDs []int64) error {
	if _, err := tx.Exec(ctx, `DELETE FROM user_group_user WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("clear user groups: %w", err)
	}
	for _, gid := range groupIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_group_user (user_group_id, user_id) VALUES ($1, $2)`,
			gid, userID,
		); err != nil {
			if strings.Contains(err.Error(), "violates foreign key") {
				return ErrConflict
			}
			return fmt.Errorf("insert user_group_user: %w", err)
		}
	}
	return nil
}

// setCollectionGrantsTx replaces u's user-scoped collection_grant rows
// with grants. Group-scoped rows are left untouched.
func setCollectionGrantsTx(ctx context.Context, tx pgx.Tx, userID int64, grants []GrantCreate) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM collection_grant WHERE user_id = $1`, userID,
	); err != nil {
		return fmt.Errorf("clear user grants: %w", err)
	}
	for _, g := range grants {
		if g.RoleID < 1 || g.RoleID > 4 {
			return errors.New("store: grant role must be 1..4")
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO collection_grant (collection_id, user_id, role_id) VALUES ($1, $2, $3)`,
			g.CollectionID, userID, g.RoleID,
		); err != nil {
			if strings.Contains(err.Error(), "violates foreign key") {
				return ErrConflict
			}
			if strings.Contains(err.Error(), "duplicate key") {
				return ErrConflict
			}
			return fmt.Errorf("insert grant: %w", err)
		}
	}
	return nil
}
