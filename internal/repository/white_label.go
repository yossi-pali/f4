package repository

import (
	"context"
	"encoding/json"

	"github.com/jmoiron/sqlx"
)

// WhiteLabelConfig holds the filter configuration for a white-label partner.
type WhiteLabelConfig struct {
	OperatorIDs []int
	CountryIDs  []string
	VehclassIDs []string
}

// WhiteLabelRepo handles white-label configuration queries.
// Ported from PHP WhiteLabel + AgentWhiteLabelRepository.
type WhiteLabelRepo struct {
	db *sqlx.DB
}

func NewWhiteLabelRepo(db *sqlx.DB) *WhiteLabelRepo {
	return &WhiteLabelRepo{db: db}
}

// wlFilters matches the JSON structure of the "filters" field in config_merged.
type wlFilters struct {
	Operator []int    `json:"operator"`
	Country  []string `json:"country"`
	Vehclass []string `json:"vehclass"`
}

// wlConfigMerged is the top-level JSON structure of the config_merged column.
type wlConfigMerged struct {
	Filters wlFilters `json:"filters"`
}

// GetConfig returns the white-label config for an agent by looking up the
// whitelabel table via the user's associated domain.
func (r *WhiteLabelRepo) GetConfig(ctx context.Context, agentID int) (WhiteLabelConfig, error) {
	var cfg WhiteLabelConfig
	if agentID <= 0 || r.db == nil {
		return cfg, nil
	}

	// Find the domain associated with this agent and get its merged config
	var configJSON *string
	err := r.db.GetContext(ctx, &configJSON, `
		SELECT w.config_merged
		FROM whitelabel w
		WHERE w.usr_id = ? AND w.state = 'ENABLED'
		LIMIT 1`, agentID)
	if err != nil || configJSON == nil || *configJSON == "" {
		// No white-label config for this agent
		return cfg, nil
	}

	var merged wlConfigMerged
	if err := json.Unmarshal([]byte(*configJSON), &merged); err != nil {
		return cfg, nil // Malformed JSON, treat as no config
	}

	cfg.OperatorIDs = merged.Filters.Operator
	cfg.CountryIDs = merged.Filters.Country
	cfg.VehclassIDs = merged.Filters.Vehclass

	return cfg, nil
}

// GetConfigByDomain returns the white-label config for a specific domain.
func (r *WhiteLabelRepo) GetConfigByDomain(ctx context.Context, domain string) (WhiteLabelConfig, error) {
	var cfg WhiteLabelConfig
	if domain == "" || r.db == nil {
		return cfg, nil
	}

	var configJSON *string
	err := r.db.GetContext(ctx, &configJSON, `
		SELECT config_merged
		FROM whitelabel
		WHERE domain = ? AND state = 'ENABLED'`, domain)
	if err != nil || configJSON == nil || *configJSON == "" {
		return cfg, nil
	}

	var merged wlConfigMerged
	if err := json.Unmarshal([]byte(*configJSON), &merged); err != nil {
		return cfg, nil
	}

	cfg.OperatorIDs = merged.Filters.Operator
	cfg.CountryIDs = merged.Filters.Country
	cfg.VehclassIDs = merged.Filters.Vehclass

	return cfg, nil
}
