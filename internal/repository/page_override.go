package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// PageOverride holds page meta override data.
type PageOverride struct {
	URL     string
	Param   string
	Value   string
}

// PageOverrideRepo handles admin page override queries.
type PageOverrideRepo struct {
	db *sqlx.DB
}

func NewPageOverrideRepo(db *sqlx.DB) *PageOverrideRepo {
	return &PageOverrideRepo{db: db}
}

// GetByURL returns all page meta overrides for a URL.
func (r *PageOverrideRepo) GetByURL(ctx context.Context, url string) (map[string]string, error) {
	var rows []struct {
		Param string `db:"param1"`
		Value string `db:"value1"`
	}
	err := r.db.SelectContext(ctx, &rows,
		`SELECT param1, value1 FROM page_override WHERE page_url = ?`, url)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(rows))
	for _, row := range rows {
		result[row.Param] = row.Value
	}
	return result, nil
}

// Set sets a page meta override.
func (r *PageOverrideRepo) Set(ctx context.Context, url, param, value string) error {
	if value == "" {
		_, err := r.db.ExecContext(ctx,
			`DELETE FROM page_override WHERE page_url = ? AND param1 = ?`, url, param)
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO page_override (page_url, param1, value1) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE value1 = VALUES(value1)`, url, param, value)
	return err
}
