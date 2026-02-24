package repository

import (
	"context"
	"fmt"
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

// OperatorRating holds rating data from the operator_extra table.
type OperatorRating struct {
	OperatorID   int     `db:"operator_id"`
	Rating       float64 `db:"rating"`
	RatingsCount int     `db:"ratings_count"`
}

// FindOperatorRatings returns operator ratings from the operator_extra table,
// matching PHP OperatorExtraRepository::getRatingsByOperatorIds().
// Uses the same weighted average calculation: (1*c1 + 2*c2 + 3*c3 + 4*c4 + 5*c5) / (c1+c2+c3+c4+c5).
func (r *OperatorRepo) FindOperatorRatings(ctx context.Context, operatorIDs []int) (map[int]OperatorRating, error) {
	if len(operatorIDs) == 0 {
		return map[int]OperatorRating{}, nil
	}

	c1 := "CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.data, '$.ratingAll1')), '0') AS DECIMAL(20,4))"
	c2 := "CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.data, '$.ratingAll2')), '0') AS DECIMAL(20,4))"
	c3 := "CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.data, '$.ratingAll3')), '0') AS DECIMAL(20,4))"
	c4 := "CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.data, '$.ratingAll4')), '0') AS DECIMAL(20,4))"
	c5 := "CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.data, '$.ratingAll5')), '0') AS DECIMAL(20,4))"

	weighted := fmt.Sprintf("(%s*1 + %s*2 + %s*3 + %s*4 + %s*5)", c1, c2, c3, c4, c5)
	total := fmt.Sprintf("(%s + %s + %s + %s + %s)", c1, c2, c3, c4, c5)

	baseQuery := fmt.Sprintf(`
		SELECT o.operator_id,
		       COALESCE(ROUND(%s / NULLIF(%s, 0), 2), 0) AS rating,
		       CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.data, '$.ratingsCount')), '0') AS UNSIGNED) AS ratings_count
		FROM operator_extra o
		WHERE o.extra_type = 'rating'
		  AND o.operator_id IN (?)`, weighted, total)

	query, args, err := sqlx.In(baseQuery, operatorIDs)
	if err != nil {
		return nil, fmt.Errorf("operator ratings IN expand: %w", err)
	}

	var rows []OperatorRating
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("operator ratings query: %w", err)
	}

	result := make(map[int]OperatorRating, len(rows))
	for _, row := range rows {
		result[row.OperatorID] = row
	}
	return result, nil
}
