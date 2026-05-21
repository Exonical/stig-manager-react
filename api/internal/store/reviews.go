package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Review is one evaluator decision (pass/fail/notapplicable/…) on a
// single (Asset × Rule) pair. The active row is updated in-place; the
// previous content is appended to review_history on every change.
type Review struct {
	ReviewID     int64
	AssetID      int64
	RuleID       string
	Result       string
	Detail       string
	Comment      string
	AutoResult   bool
	ResultEngine json.RawMessage
	Metadata     json.RawMessage
	StatusLabel  string
	StatusText   string
	StatusUserID *int64
	StatusTS     time.Time
	UserID       int64
	Username     string
	TS           time.Time
	TouchTS      time.Time
}

// ReviewHistoryEntry is a previous version of a Review.
type ReviewHistoryEntry struct {
	HistoryID    int64
	ReviewID     *int64
	AssetID      int64
	RuleID       string
	Result       string
	Detail       string
	Comment      string
	AutoResult   bool
	ResultEngine json.RawMessage
	StatusLabel  string
	StatusText   string
	UserID       int64
	Username     string
	TS           time.Time
	TouchTS      time.Time
}

// ReviewWrite is a complete-replace payload for ReviewRepo.Put.
type ReviewWrite struct {
	Result       string
	Detail       string
	Comment      string
	AutoResult   bool
	ResultEngine json.RawMessage
	Metadata     json.RawMessage
	StatusLabel  string
	StatusText   string
}

// ReviewPatch is a merge-only payload for ReviewRepo.Patch. Non-nil
// pointer fields are written; nil pointers are left untouched.
type ReviewPatch struct {
	Result       *string
	Detail       *string
	Comment      *string
	AutoResult   *bool
	ResultEngine json.RawMessage // nil = leave; len()==0 (after json.RawMessage("null")) = clear is not supported here
	Metadata     json.RawMessage
	StatusLabel  *string
	StatusText   *string
}

// ListReviewsOptions narrows a review listing. CollectionID is
// required; other fields are optional filters.
type ListReviewsOptions struct {
	CollectionID int64
	AssetID      int64
	RuleID       string
	Result       string
	Status       string
}

// ReviewRepo provides CRUD for individual asset-rule reviews and their
// append-only history.
type ReviewRepo struct {
	pool *pgxpool.Pool
}

// NewReviewRepo returns a ReviewRepo backed by pool.
func NewReviewRepo(pool *pgxpool.Pool) *ReviewRepo {
	return &ReviewRepo{pool: pool}
}

// Get returns a single Review by (assetID, ruleID). Returns ErrNotFound
// when no row exists.
func (r *ReviewRepo) Get(ctx context.Context, assetID int64, ruleID string) (Review, error) {
	const q = `
SELECT r.review_id, r.asset_id, r.rule_id, r.result,
       r.detail, r.comment, r.auto_result, r.result_engine, r.metadata,
       r.status_label, COALESCE(r.status_text,''), r.status_user_id, r.status_ts,
       r.user_id, COALESCE(u.username,''), r.ts, r.touch_ts
FROM review r
LEFT JOIN app_user u ON u.user_id = r.user_id
WHERE r.asset_id = $1 AND r.rule_id = $2
`
	var out Review
	if err := r.pool.QueryRow(ctx, q, assetID, ruleID).Scan(
		&out.ReviewID, &out.AssetID, &out.RuleID, &out.Result,
		&out.Detail, &out.Comment, &out.AutoResult, &out.ResultEngine, &out.Metadata,
		&out.StatusLabel, &out.StatusText, &out.StatusUserID, &out.StatusTS,
		&out.UserID, &out.Username, &out.TS, &out.TouchTS,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Review{}, ErrNotFound
		}
		return Review{}, fmt.Errorf("get review: %w", err)
	}
	return out, nil
}

// List returns all Reviews for the given collection that match the
// supplied filters. Ordered by asset name then rule_id for a stable
// HTTP output.
func (r *ReviewRepo) List(ctx context.Context, opt ListReviewsOptions) ([]Review, error) {
	if opt.CollectionID == 0 {
		return nil, errors.New("store: list reviews: collection id required")
	}
	args := []any{opt.CollectionID}
	conds := []string{"a.collection_id = $1", "a.state = 'enabled'"}
	if opt.AssetID > 0 {
		args = append(args, opt.AssetID)
		conds = append(conds, fmt.Sprintf("r.asset_id = $%d", len(args)))
	}
	if opt.RuleID != "" {
		args = append(args, opt.RuleID)
		conds = append(conds, fmt.Sprintf("r.rule_id = $%d", len(args)))
	}
	if opt.Result != "" {
		args = append(args, opt.Result)
		conds = append(conds, fmt.Sprintf("r.result = $%d", len(args)))
	}
	if opt.Status != "" {
		args = append(args, opt.Status)
		conds = append(conds, fmt.Sprintf("r.status_label = $%d", len(args)))
	}

	q := `
SELECT r.review_id, r.asset_id, r.rule_id, r.result,
       r.detail, r.comment, r.auto_result, r.result_engine, r.metadata,
       r.status_label, COALESCE(r.status_text,''), r.status_user_id, r.status_ts,
       r.user_id, COALESCE(u.username,''), r.ts, r.touch_ts
FROM review r
JOIN asset a ON a.asset_id = r.asset_id
LEFT JOIN app_user u ON u.user_id = r.user_id
WHERE ` + strings.Join(conds, " AND ") + `
ORDER BY lower(a.name) ASC, r.rule_id ASC`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	defer rows.Close()
	out := []Review{}
	for rows.Next() {
		var rv Review
		if err := rows.Scan(
			&rv.ReviewID, &rv.AssetID, &rv.RuleID, &rv.Result,
			&rv.Detail, &rv.Comment, &rv.AutoResult, &rv.ResultEngine, &rv.Metadata,
			&rv.StatusLabel, &rv.StatusText, &rv.StatusUserID, &rv.StatusTS,
			&rv.UserID, &rv.Username, &rv.TS, &rv.TouchTS,
		); err != nil {
			return nil, fmt.Errorf("scan review: %w", err)
		}
		out = append(out, rv)
	}
	return out, rows.Err()
}

// Put inserts or fully-replaces the Review for (assetID, ruleID) with
// the supplied payload. If a row already exists, its previous content
// is snapshotted to review_history before the update.
func (r *ReviewRepo) Put(ctx context.Context, assetID int64, ruleID string, userID int64, in ReviewWrite) (Review, error) {
	if strings.TrimSpace(in.Result) == "" {
		return Review{}, errors.New("store: review: result required")
	}
	statusLabel := in.StatusLabel
	if statusLabel == "" {
		statusLabel = "saved"
	}
	metadata := in.Metadata
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}
	resultEngine := in.ResultEngine
	if len(resultEngine) == 0 {
		resultEngine = nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Review{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := snapshotReviewLocked(ctx, tx, assetID, ruleID); err != nil {
		return Review{}, err
	}

	const upsert = `
INSERT INTO review (
    asset_id, rule_id, result, detail, comment, auto_result,
    result_engine, metadata,
    status_label, status_text, status_user_id, status_ts,
    user_id, ts, touch_ts
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8::jsonb,
    $9, NULLIF($10, ''), $11, now(),
    $11, now(), now()
)
ON CONFLICT (asset_id, rule_id) DO UPDATE SET
    result        = EXCLUDED.result,
    detail        = EXCLUDED.detail,
    comment       = EXCLUDED.comment,
    auto_result   = EXCLUDED.auto_result,
    result_engine = EXCLUDED.result_engine,
    metadata      = EXCLUDED.metadata,
    status_label  = EXCLUDED.status_label,
    status_text   = EXCLUDED.status_text,
    status_user_id = EXCLUDED.status_user_id,
    status_ts     = EXCLUDED.status_ts,
    user_id       = EXCLUDED.user_id,
    ts            = EXCLUDED.ts,
    touch_ts      = EXCLUDED.touch_ts
RETURNING review_id, status_ts, ts, touch_ts
`
	var (
		reviewID int64
		statusTS time.Time
		ts       time.Time
		touchTS  time.Time
	)
	if err := tx.QueryRow(ctx, upsert,
		assetID, ruleID, in.Result, in.Detail, in.Comment, in.AutoResult,
		resultEngine, metadata,
		statusLabel, in.StatusText, userID,
	).Scan(&reviewID, &statusTS, &ts, &touchTS); err != nil {
		return Review{}, fmt.Errorf("upsert review: %w", classifyReviewErr(err))
	}

	if err := tx.Commit(ctx); err != nil {
		return Review{}, fmt.Errorf("commit: %w", err)
	}
	return r.Get(ctx, assetID, ruleID)
}

// Patch merges the non-nil fields of in into the existing Review.
// Returns ErrNotFound if no review currently exists for the pair.
func (r *ReviewRepo) Patch(ctx context.Context, assetID int64, ruleID string, userID int64, in ReviewPatch) (Review, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Review{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Confirm the row exists and snapshot it for history.
	if err := snapshotReviewLocked(ctx, tx, assetID, ruleID); err != nil {
		return Review{}, err
	}
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM review WHERE asset_id=$1 AND rule_id=$2)`,
		assetID, ruleID,
	).Scan(&exists); err != nil {
		return Review{}, fmt.Errorf("exists: %w", err)
	}
	if !exists {
		return Review{}, ErrNotFound
	}

	args := []any{assetID, ruleID}
	sets := []string{"user_id = $3", "ts = now()", "touch_ts = now()"}
	args = append(args, userID)

	if in.Result != nil {
		args = append(args, *in.Result)
		sets = append(sets, fmt.Sprintf("result = $%d", len(args)))
	}
	if in.Detail != nil {
		args = append(args, *in.Detail)
		sets = append(sets, fmt.Sprintf("detail = $%d", len(args)))
	}
	if in.Comment != nil {
		args = append(args, *in.Comment)
		sets = append(sets, fmt.Sprintf("comment = $%d", len(args)))
	}
	if in.AutoResult != nil {
		args = append(args, *in.AutoResult)
		sets = append(sets, fmt.Sprintf("auto_result = $%d", len(args)))
	}
	if in.ResultEngine != nil {
		args = append(args, in.ResultEngine)
		sets = append(sets, fmt.Sprintf("result_engine = $%d::jsonb", len(args)))
	}
	if in.Metadata != nil {
		args = append(args, in.Metadata)
		sets = append(sets, fmt.Sprintf("metadata = $%d::jsonb", len(args)))
	}
	if in.StatusLabel != nil {
		args = append(args, *in.StatusLabel)
		sets = append(sets, fmt.Sprintf("status_label = $%d", len(args)))
		args = append(args, userID)
		sets = append(sets, fmt.Sprintf("status_user_id = $%d", len(args)))
		sets = append(sets, "status_ts = now()")
	}
	if in.StatusText != nil {
		args = append(args, *in.StatusText)
		sets = append(sets, fmt.Sprintf("status_text = NULLIF($%d, '')", len(args)))
	}

	q := fmt.Sprintf(`UPDATE review SET %s WHERE asset_id = $1 AND rule_id = $2`,
		strings.Join(sets, ", "))
	if _, err := tx.Exec(ctx, q, args...); err != nil {
		return Review{}, fmt.Errorf("patch review: %w", classifyReviewErr(err))
	}

	if err := tx.Commit(ctx); err != nil {
		return Review{}, fmt.Errorf("commit: %w", err)
	}
	return r.Get(ctx, assetID, ruleID)
}

// Delete removes the Review for (assetID, ruleID) after snapshotting
// it to history. Returns ErrNotFound when the review does not exist.
func (r *ReviewRepo) Delete(ctx context.Context, assetID int64, ruleID string) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := snapshotReviewLocked(ctx, tx, assetID, ruleID); err != nil {
		return err
	}
	ct, err := tx.Exec(ctx,
		`DELETE FROM review WHERE asset_id = $1 AND rule_id = $2`,
		assetID, ruleID,
	)
	if err != nil {
		return fmt.Errorf("delete review: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

// History returns the append-only history for a (assetID, ruleID)
// pair, newest-first.
func (r *ReviewRepo) History(ctx context.Context, assetID int64, ruleID string) ([]ReviewHistoryEntry, error) {
	const q = `
SELECT h.history_id, h.review_id, h.asset_id, h.rule_id, h.result,
       h.detail, h.comment, h.auto_result, h.result_engine,
       h.status_label, COALESCE(h.status_text,''),
       h.user_id, COALESCE(u.username,''), h.ts, h.touch_ts
FROM review_history h
LEFT JOIN app_user u ON u.user_id = h.user_id
WHERE h.asset_id = $1 AND h.rule_id = $2
ORDER BY h.created_at DESC, h.history_id DESC`
	rows, err := r.pool.Query(ctx, q, assetID, ruleID)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	defer rows.Close()
	out := []ReviewHistoryEntry{}
	for rows.Next() {
		var h ReviewHistoryEntry
		if err := rows.Scan(
			&h.HistoryID, &h.ReviewID, &h.AssetID, &h.RuleID, &h.Result,
			&h.Detail, &h.Comment, &h.AutoResult, &h.ResultEngine,
			&h.StatusLabel, &h.StatusText,
			&h.UserID, &h.Username, &h.TS, &h.TouchTS,
		); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// CollectionForAsset returns the collection_id that owns an asset.
// Returns ErrNotFound if the asset is missing or disabled.
func (r *ReviewRepo) CollectionForAsset(ctx context.Context, assetID int64) (int64, error) {
	var cid int64
	err := r.pool.QueryRow(ctx,
		`SELECT collection_id FROM asset WHERE asset_id = $1 AND state = 'enabled'`,
		assetID,
	).Scan(&cid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("collection for asset: %w", err)
	}
	return cid, nil
}

// snapshotReviewLocked copies the current review row (if any) into
// review_history. The SELECT FOR UPDATE serialises concurrent writers
// so two simultaneous PATCH requests can't drop history entries.
func snapshotReviewLocked(ctx context.Context, tx pgx.Tx, assetID int64, ruleID string) error {
	const snap = `
INSERT INTO review_history (
    review_id, asset_id, rule_id, result, detail, comment, auto_result,
    result_engine, status_label, status_text, user_id, ts, touch_ts
)
SELECT review_id, asset_id, rule_id, result, detail, comment, auto_result,
       result_engine, status_label, status_text, user_id, ts, touch_ts
FROM review
WHERE asset_id = $1 AND rule_id = $2
FOR UPDATE`
	if _, err := tx.Exec(ctx, snap, assetID, ruleID); err != nil {
		return fmt.Errorf("snapshot review: %w", err)
	}
	return nil
}

// classifyReviewErr maps Postgres errors that handlers want to surface
// distinctly (uniqueness, foreign-key, check constraints) onto store
// sentinel errors.
func classifyReviewErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "violates check constraint"):
		return ErrConflict
	case strings.Contains(msg, "violates foreign key constraint"):
		return ErrConflict
	}
	return err
}
