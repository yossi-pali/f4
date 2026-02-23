package repository

import (
	"context"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/12go/f4/internal/domain"
)

// slugifyOperatorName computes the operator slug from name, matching PHP Slugger::slug().
// PHP applies sequential str_replace: lowercase, then specific char replacements in order.
func slugifyOperatorName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	// PHP str_replace processes these sequentially (each on the result of the previous)
	replacements := []struct{ old, new string }{
		{"=", "|"},
		{" - ", "="},
		{"-", "_"},
		{"#", "-23-"},
		{"$", "-24-"},
		{"%", "-25-"},
		{"&", "-26-"},
		{"?", "-3f-"},
		{" ", "-"},
	}
	for _, r := range replacements {
		s = strings.ReplaceAll(s, r.old, r.new)
	}
	return s
}

// operatorRow holds raw DB columns before computing derived fields.
type operatorRow struct {
	OperatorID    int     `db:"operator_id"`
	Name          string  `db:"operator_name"`
	SlugDB        string  `db:"slug"`
	SellerID      int     `db:"seller_id"`
	MasterID      int     `db:"master_id"`
	Bookable      int     `db:"bookable"`
	Code          *string `db:"operator_code"`
	CounterpartID int     `db:"counterpart_id"`
}

func (row operatorRow) toOperator() domain.Operator {
	return domain.Operator{
		OperatorID:    row.OperatorID,
		Name:          row.Name,
		Slug:          slugifyOperatorName(row.Name), // computed like PHP, not from DB
		SellerID:      row.SellerID,
		MasterID:      row.MasterID,
		Bookable:      row.Bookable != 0,
		Code:          row.Code,
		CounterpartID: row.CounterpartID,
	}
}

// OperatorRepo handles operator and seller queries.
type OperatorRepo struct {
	db *sqlx.DB
}

func NewOperatorRepo(db *sqlx.DB) *OperatorRepo {
	return &OperatorRepo{db: db}
}

// FindByIDs returns operators by IDs as a map.
func (r *OperatorRepo) FindByIDs(ctx context.Context, ids []int) (map[int]domain.Operator, error) {
	if len(ids) == 0 {
		return map[int]domain.Operator{}, nil
	}
	query, args, err := sqlx.In(`
		SELECT o.operator_id, COALESCE(o.operator_name, '') AS operator_name,
		       COALESCE(o.slug, '') AS slug, COALESCE(o.seller_id, 0) AS seller_id,
		       COALESCE(o.master_id, 0) AS master_id, COALESCE(o.bookable, 0) AS bookable,
		       o.operator_code,
		       COALESCE(s.counterpart_id, 0) AS counterpart_id
		FROM operator o
		LEFT JOIN seller s ON s.seller_id = o.seller_id
		WHERE o.operator_id IN (?)`, ids)
	if err != nil {
		return nil, err
	}

	var rows []operatorRow
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, err
	}

	result := make(map[int]domain.Operator, len(rows))
	for _, row := range rows {
		op := row.toOperator()
		result[op.OperatorID] = op
	}
	return result, nil
}
