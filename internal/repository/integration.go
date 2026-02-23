package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// Integration represents an integration partner.
type Integration struct {
	IntegrationID   int    `db:"integration_id"`
	IntegrationCode string `db:"integration_code"`
	SellerID        int    `db:"seller_id"`
	ChunkKey        string `db:"chunk_key"`
}

// IntegrationRepo handles integration metadata queries.
type IntegrationRepo struct {
	db *sqlx.DB
}

func NewIntegrationRepo(db *sqlx.DB) *IntegrationRepo {
	return &IntegrationRepo{db: db}
}

// FindByCode returns an integration by code.
func (r *IntegrationRepo) FindByCode(ctx context.Context, code string) (Integration, error) {
	var i Integration
	err := r.db.GetContext(ctx, &i,
		`SELECT integration_id, integration_code, seller_id, chunk_key FROM integration WHERE integration_code = ?`, code)
	return i, err
}

// FindBySellerID returns an integration by seller ID.
func (r *IntegrationRepo) FindBySellerID(ctx context.Context, sellerID int) (Integration, error) {
	var i Integration
	err := r.db.GetContext(ctx, &i,
		`SELECT integration_id, integration_code, seller_id, chunk_key FROM integration WHERE seller_id = ?`, sellerID)
	return i, err
}
