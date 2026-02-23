package repository

import (
	"context"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
)

// SecurityRestrictions holds include/exclude lists for an agent.
type SecurityRestrictions struct {
	OperatorIDs        []int
	SellerIDs          []int
	CountryIDs         []string
	VehclassIDs        []string
	ClassIDs           []int
	ExcludeOperatorIDs []int
	ExcludeSellerIDs   []int
	ExcludeCountryIDs  []string
	ExcludeVehclassIDs []string
	ExcludeClassIDs    []int
}

// searchSecureFields are the data_sec object types used for search filtering.
var searchSecureFields = []string{
	"operator_id",
	"seller_id",
	"country_id",
	"vehclass_id",
	"class_id",
}

// DataSecRepo handles security restriction queries.
// Ported from PHP DataSecManager + DataSecRepository.
type DataSecRepo struct {
	db *sqlx.DB
}

func NewDataSecRepo(db *sqlx.DB) *DataSecRepo {
	return &DataSecRepo{db: db}
}

type dataSecRow struct {
	Type     string `db:"type"`
	Object   string `db:"object"`
	ObjectID string `db:"object_id"`
	Access   string `db:"access"`
}

// GetRestrictions returns security restrictions for an agent.
// Queries both data_sec (user-level) and data_sec_role (role-level) tables.
func (r *DataSecRepo) GetRestrictions(ctx context.Context, agentID int) (SecurityRestrictions, error) {
	var sec SecurityRestrictions
	if agentID <= 0 || r.db == nil {
		return sec, nil
	}

	query, args, err := sqlx.In(`
		SELECT 'usr_id' AS type, object, object_id, access
		FROM data_sec
		WHERE usr_id = ? AND object IN (?)
		UNION ALL
		SELECT 'role_id' AS type, dsr.object, dsr.object_id, dsr.access
		FROM data_sec_role dsr
		JOIN usr u ON u.role_id = dsr.role_id
		WHERE u.usr_id = ? AND dsr.object IN (?)`,
		agentID, searchSecureFields,
		agentID, searchSecureFields,
	)
	if err != nil {
		return sec, err
	}

	var rows []dataSecRow
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return sec, err
	}

	// Build restrictions: user-level entries take precedence (first match wins per key)
	type restrictionKey struct {
		object   string
		objectID string
	}
	seen := make(map[restrictionKey]bool)

	for _, row := range rows {
		key := restrictionKey{row.Object, row.ObjectID}
		if seen[key] {
			continue
		}
		seen[key] = true

		ids := strings.Split(row.ObjectID, ",")

		switch row.Object {
		case "operator_id":
			intIDs := toInts(ids)
			if row.Access == "D" {
				sec.ExcludeOperatorIDs = append(sec.ExcludeOperatorIDs, intIDs...)
			} else {
				sec.OperatorIDs = append(sec.OperatorIDs, intIDs...)
			}
		case "seller_id":
			intIDs := toInts(ids)
			if row.Access == "D" {
				sec.ExcludeSellerIDs = append(sec.ExcludeSellerIDs, intIDs...)
			} else {
				sec.SellerIDs = append(sec.SellerIDs, intIDs...)
			}
		case "country_id":
			trimmed := trimStrings(ids)
			if row.Access == "D" {
				sec.ExcludeCountryIDs = append(sec.ExcludeCountryIDs, trimmed...)
			} else {
				sec.CountryIDs = append(sec.CountryIDs, trimmed...)
			}
		case "vehclass_id":
			trimmed := trimStrings(ids)
			if row.Access == "D" {
				sec.ExcludeVehclassIDs = append(sec.ExcludeVehclassIDs, trimmed...)
			} else {
				sec.VehclassIDs = append(sec.VehclassIDs, trimmed...)
			}
		case "class_id":
			intIDs := toInts(ids)
			if row.Access == "D" {
				sec.ExcludeClassIDs = append(sec.ExcludeClassIDs, intIDs...)
			} else {
				sec.ClassIDs = append(sec.ClassIDs, intIDs...)
			}
		}
	}

	return sec, nil
}

func toInts(ss []string) []int {
	var result []int
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if v, err := strconv.Atoi(s); err == nil {
			result = append(result, v)
		}
	}
	return result
}

func trimStrings(ss []string) []string {
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}
