package repository

import (
	"context"
	"strings"

	"github.com/jmoiron/sqlx"
)

// TranRepo handles translation lookups from the tran_v table.
type TranRepo struct {
	db *sqlx.DB
}

func NewTranRepo(db *sqlx.DB) *TranRepo {
	return &TranRepo{db: db}
}

// phraseTranID computes the tran_id from a phrase, matching PHP Text::getPhraseTranId.
func phraseTranID(phrase string) string {
	s := strings.ToLower(phrase)
	if len(s) > 100 {
		s = s[:100]
	}
	return strings.ReplaceAll(s, ",", "")
}

// TranslateMany translates multiple phrases using the tran_v table.
// Returns a map of original phrase → translated phrase. Untranslated phrases map to themselves.
func (r *TranRepo) TranslateMany(ctx context.Context, phrases []string, lang string) (map[string]string, error) {
	if len(phrases) == 0 || r.db == nil {
		return map[string]string{}, nil
	}

	// Build tran_id → original phrase mapping
	idToPhrase := make(map[string]string, len(phrases))
	tranIDs := make([]string, 0, len(phrases))
	for _, p := range phrases {
		if p == "" {
			continue
		}
		tid := phraseTranID(p)
		if _, exists := idToPhrase[tid]; !exists {
			idToPhrase[tid] = p
			tranIDs = append(tranIDs, tid)
		}
	}

	if len(tranIDs) == 0 {
		return map[string]string{}, nil
	}

	// Query translations: select tran_id and tran_{lang} column
	// PHP uses tran_{lang} (e.g., tran_en) with fallback to tran column
	langCol := "tran_" + lang
	query, args, err := sqlx.In(
		`SELECT tran_id, COALESCE(`+langCol+`, tran, '') AS translated
		 FROM tran_v
		 WHERE tran_id IN (?) AND tran_dom = '12go.v2'`, tranIDs)
	if err != nil {
		return nil, err
	}

	var rows []struct {
		TranID     string `db:"tran_id"`
		Translated string `db:"translated"`
	}
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, err
	}

	// Build result: original phrase → translated (or original if no translation)
	result := make(map[string]string, len(phrases))
	for _, row := range rows {
		if row.Translated != "" {
			if orig, ok := idToPhrase[row.TranID]; ok {
				result[orig] = row.Translated
			}
		}
	}

	return result, nil
}
