package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

// ReasonRepo loads reason texts from the trip_unavailable_reason + tran tables.
type ReasonRepo struct {
	db *sqlx.DB
}

func NewReasonRepo(db *sqlx.DB) *ReasonRepo {
	return &ReasonRepo{db: db}
}

// FindReasonTexts returns a map of reason_id → translated text for the given lang.
// Matches PHP ReasonCollector which queries trip_unavailable_reason → tran_v.
func (r *ReasonRepo) FindReasonTexts(ctx context.Context, ids []int, lang string) (map[int]string, error) {
	if len(ids) == 0 {
		return map[int]string{}, nil
	}

	col := "tran_en" // default
	if lang != "" {
		col = fmt.Sprintf("tran_%s", lang)
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT r.reason_id, COALESCE(t.%s, t.tran_en, '') AS reason_text
		FROM trip_unavailable_reason r
		JOIN tran_v t ON t.tran_id = r.tran_id AND t.tran_dom = '12go.v2'
		WHERE r.reason_id IN (%s)`,
		col, strings.Join(placeholders, ","))

	type row struct {
		ReasonID   int    `db:"reason_id"`
		ReasonText string `db:"reason_text"`
	}

	var rows []row
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("reason texts: %w", err)
	}

	result := make(map[int]string, len(rows))
	for _, r := range rows {
		result[r.ReasonID] = r.ReasonText
	}
	return result, nil
}
