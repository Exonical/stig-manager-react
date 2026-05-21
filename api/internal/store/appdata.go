package store

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AppDataRepo exports/imports the contents of the application tables
// as a single JSON document. Only the export half is implemented in
// this milestone; ReplaceAppData is staged for a follow-up because it
// requires FK-aware ordering and conflict resolution.
type AppDataRepo struct {
	pool *pgxpool.Pool
}

// NewAppDataRepo wires an AppDataRepo to the given pool.
func NewAppDataRepo(pool *pgxpool.Pool) *AppDataRepo { return &AppDataRepo{pool: pool} }

// ExportTables is the ordered list of public tables included in the
// JSON dump. Order matters for any future import — children follow
// their parents.
var ExportTables = []string{
	"app_user",
	"user_group",
	"user_group_user",
	"collection",
	"collection_grant",
	"asset",
	"stig",
	"stig_revision",
	"stig_rule",
	"cci",
	"stig_rule_cci",
	"asset_stig",
	"collection_label",
	"collection_label_asset",
	"review",
	"review_history",
	"job_task",
	"job",
	"job_task_link",
	"job_run",
	"job_run_output",
}

// Export writes a JSON document of the form
//
//	{ "tables": { "<name>": [ {row}, {row}, ... ], ... } }
//
// to w. Streaming is used so a large export never has to fit in
// memory. Caller is responsible for HTTP headers.
func (r *AppDataRepo) Export(ctx context.Context, w io.Writer) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("no pool")
	}
	if _, err := io.WriteString(w, `{"tables":{`); err != nil {
		return err
	}
	for i, t := range ExportTables {
		if i > 0 {
			if _, err := io.WriteString(w, ","); err != nil {
				return err
			}
		}
		nameBytes, err := json.Marshal(t)
		if err != nil {
			return err
		}
		if _, err := w.Write(nameBytes); err != nil {
			return err
		}
		if _, err := io.WriteString(w, ":"); err != nil {
			return err
		}
		if err := r.exportTable(ctx, w, t); err != nil {
			return fmt.Errorf("export %s: %w", t, err)
		}
	}
	_, err := io.WriteString(w, `}}`)
	return err
}

func (r *AppDataRepo) exportTable(ctx context.Context, w io.Writer, name string) error {
	q := fmt.Sprintf("SELECT row_to_json(t) FROM %s t", name)
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	if _, err := io.WriteString(w, "["); err != nil {
		return err
	}
	first := true
	for rows.Next() {
		var rowJSON []byte
		if err := rows.Scan(&rowJSON); err != nil {
			return err
		}
		if !first {
			if _, err := io.WriteString(w, ","); err != nil {
				return err
			}
		}
		first = false
		if _, err := w.Write(rowJSON); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = io.WriteString(w, "]")
	return err
}
