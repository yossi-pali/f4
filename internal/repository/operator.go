package repository

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/12go/f4/internal/domain"
)

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
		SELECT operator_id, COALESCE(operator_name, '') AS operator_name,
		       COALESCE(slug, '') AS slug, COALESCE(seller_id, 0) AS seller_id,
		       COALESCE(master_id, 0) AS master_id, COALESCE(bookable, 0) AS bookable
		FROM operator
		WHERE operator_id IN (?)`, ids)
	if err != nil {
		return nil, err
	}

	var operators []domain.Operator
	if err := r.db.SelectContext(ctx, &operators, r.db.Rebind(query), args...); err != nil {
		return nil, err
	}

	result := make(map[int]domain.Operator, len(operators))
	for _, op := range operators {
		result[op.OperatorID] = op
	}
	return result, nil
}
