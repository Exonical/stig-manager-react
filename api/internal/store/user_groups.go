package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserGroup is the admin-side projection used by /user-groups CRUD.
type UserGroup struct {
	UserGroupID      int64
	Name             string
	Description      string
	Users            []UserBasic
	CollectionGrants []UserCollectionGrant
}

// UserBasic is the {user_id, username, display_name} triplet returned
// inside a UserGroup projection.
type UserBasic struct {
	UserID      int64
	Username    string
	DisplayName string
}

// UserGroupCreate is the input bundle for CreateAdmin (mirrors the
// UserGroupPostOrPut OpenAPI shape).
type UserGroupCreate struct {
	Name             string
	Description      string
	UserIDs          []int64
	CollectionGrants []GrantCreate
}

// UserGroupPatch is the input bundle for PatchAdmin.
type UserGroupPatch struct {
	Name             *string
	Description      *string
	UserIDs          *[]int64
	CollectionGrants *[]GrantCreate
}

// UserGroupRepo provides CRUD for user_group rows.
type UserGroupRepo struct {
	pool *pgxpool.Pool
}

// NewUserGroupRepo returns a UserGroupRepo backed by pool.
func NewUserGroupRepo(pool *pgxpool.Pool) *UserGroupRepo {
	return &UserGroupRepo{pool: pool}
}

// List returns all user groups, ordered by name.
func (r *UserGroupRepo) List(ctx context.Context) ([]UserGroup, error) {
	const q = `
SELECT user_group_id, name, COALESCE(description, '')
FROM user_group
ORDER BY lower(name) ASC
`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list user groups: %w", err)
	}
	defer rows.Close()
	var out []UserGroup
	for rows.Next() {
		var g UserGroup
		if err := rows.Scan(&g.UserGroupID, &g.Name, &g.Description); err != nil {
			return nil, fmt.Errorf("scan user group: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Get returns one group with users + collection grants joined.
func (r *UserGroupRepo) Get(ctx context.Context, userGroupID int64) (UserGroup, error) {
	const q = `
SELECT user_group_id, name, COALESCE(description, '')
FROM user_group
WHERE user_group_id = $1
`
	var g UserGroup
	if err := r.pool.QueryRow(ctx, q, userGroupID).Scan(
		&g.UserGroupID, &g.Name, &g.Description,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserGroup{}, ErrNotFound
		}
		return UserGroup{}, fmt.Errorf("get user group: %w", err)
	}
	if err := r.attach(ctx, &g); err != nil {
		return UserGroup{}, err
	}
	return g, nil
}

// Create inserts a new user_group + member/grant rows in one
// transaction.
func (r *UserGroupRepo) Create(ctx context.Context, in UserGroupCreate) (UserGroup, error) {
	if in.Name == "" {
		return UserGroup{}, errors.New("store: user group name required")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return UserGroup{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var gid int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO user_group (name, description) VALUES ($1, $2) RETURNING user_group_id`,
		in.Name, nullableStr(in.Description),
	).Scan(&gid); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return UserGroup{}, ErrConflict
		}
		return UserGroup{}, fmt.Errorf("insert user group: %w", err)
	}
	if err := setUserGroupMembersTx(ctx, tx, gid, in.UserIDs); err != nil {
		return UserGroup{}, err
	}
	if err := setUserGroupGrantsTx(ctx, tx, gid, in.CollectionGrants); err != nil {
		return UserGroup{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UserGroup{}, fmt.Errorf("commit tx: %w", err)
	}
	return r.Get(ctx, gid)
}

// Patch applies a partial update.
func (r *UserGroupRepo) Patch(ctx context.Context, userGroupID int64, p UserGroupPatch) (UserGroup, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return UserGroup{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	setClauses := []string{}
	args := []any{}
	idx := 1
	if p.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", idx))
		args = append(args, *p.Name)
		idx++
	}
	if p.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", idx))
		args = append(args, nullableStr(*p.Description))
		idx++
	}
	if len(setClauses) > 0 {
		q := "UPDATE user_group SET " + strings.Join(setClauses, ", ") +
			fmt.Sprintf(" WHERE user_group_id = $%d", idx)
		args = append(args, userGroupID)
		ct, err := tx.Exec(ctx, q, args...)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") {
				return UserGroup{}, ErrConflict
			}
			return UserGroup{}, fmt.Errorf("update user group: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return UserGroup{}, ErrNotFound
		}
	} else {
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT true FROM user_group WHERE user_group_id = $1`, userGroupID,
		).Scan(&exists); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return UserGroup{}, ErrNotFound
			}
			return UserGroup{}, err
		}
	}
	if p.UserIDs != nil {
		if err := setUserGroupMembersTx(ctx, tx, userGroupID, *p.UserIDs); err != nil {
			return UserGroup{}, err
		}
	}
	if p.CollectionGrants != nil {
		if err := setUserGroupGrantsTx(ctx, tx, userGroupID, *p.CollectionGrants); err != nil {
			return UserGroup{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return UserGroup{}, fmt.Errorf("commit tx: %w", err)
	}
	return r.Get(ctx, userGroupID)
}

// Delete removes a user_group (cascades to user_group_user and to
// collection_grant rows scoped to this group).
func (r *UserGroupRepo) Delete(ctx context.Context, userGroupID int64) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM user_group WHERE user_group_id = $1`, userGroupID)
	if err != nil {
		return fmt.Errorf("delete user group: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *UserGroupRepo) attach(ctx context.Context, g *UserGroup) error {
	mems, err := r.members(ctx, g.UserGroupID)
	if err != nil {
		return err
	}
	g.Users = mems
	grants, err := r.grants(ctx, g.UserGroupID)
	if err != nil {
		return err
	}
	g.CollectionGrants = grants
	return nil
}

func (r *UserGroupRepo) members(ctx context.Context, userGroupID int64) ([]UserBasic, error) {
	const q = `
SELECT au.user_id, au.username, au.display_name
FROM user_group_user ugu
JOIN app_user au ON au.user_id = ugu.user_id
WHERE ugu.user_group_id = $1
ORDER BY lower(au.username) ASC
`
	rows, err := r.pool.Query(ctx, q, userGroupID)
	if err != nil {
		return nil, fmt.Errorf("user group members: %w", err)
	}
	defer rows.Close()
	var out []UserBasic
	for rows.Next() {
		var u UserBasic
		if err := rows.Scan(&u.UserID, &u.Username, &u.DisplayName); err != nil {
			return nil, fmt.Errorf("scan user group member: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *UserGroupRepo) grants(ctx context.Context, userGroupID int64) ([]UserCollectionGrant, error) {
	const q = `
SELECT cg.grant_id, cg.collection_id, c.name, cg.role_id
FROM collection_grant cg
JOIN collection c ON c.collection_id = cg.collection_id
WHERE cg.user_group_id = $1
ORDER BY lower(c.name) ASC
`
	rows, err := r.pool.Query(ctx, q, userGroupID)
	if err != nil {
		return nil, fmt.Errorf("user group grants: %w", err)
	}
	defer rows.Close()
	var out []UserCollectionGrant
	for rows.Next() {
		var g UserCollectionGrant
		if err := rows.Scan(&g.GrantID, &g.CollectionID, &g.CollectionName, &g.RoleID); err != nil {
			return nil, fmt.Errorf("scan user group grant: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func setUserGroupMembersTx(ctx context.Context, tx pgx.Tx, userGroupID int64, userIDs []int64) error {
	if _, err := tx.Exec(ctx, `DELETE FROM user_group_user WHERE user_group_id = $1`, userGroupID); err != nil {
		return fmt.Errorf("clear user_group_user: %w", err)
	}
	for _, uid := range userIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_group_user (user_group_id, user_id) VALUES ($1, $2)`,
			userGroupID, uid,
		); err != nil {
			if strings.Contains(err.Error(), "violates foreign key") {
				return ErrConflict
			}
			return fmt.Errorf("insert user_group_user: %w", err)
		}
	}
	return nil
}

func setUserGroupGrantsTx(ctx context.Context, tx pgx.Tx, userGroupID int64, grants []GrantCreate) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM collection_grant WHERE user_group_id = $1`, userGroupID,
	); err != nil {
		return fmt.Errorf("clear user_group grants: %w", err)
	}
	for _, g := range grants {
		if g.RoleID < 1 || g.RoleID > 4 {
			return errors.New("store: grant role must be 1..4")
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO collection_grant (collection_id, user_group_id, role_id) VALUES ($1, $2, $3)`,
			g.CollectionID, userGroupID, g.RoleID,
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

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
