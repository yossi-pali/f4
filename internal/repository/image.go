package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/12go/f4/internal/domain"
)

// ImageRepo handles image queries for operators, classes, and routes.
type ImageRepo struct {
	db *sqlx.DB
}

func NewImageRepo(db *sqlx.DB) *ImageRepo {
	return &ImageRepo{db: db}
}

// logoRow is the scan target for operator logo queries.
type logoRow struct {
	Path       string `db:"path"`
	Width      int    `db:"width"`
	Height     int    `db:"height"`
	OperatorID int    `db:"operator_id"`
}

// FindOperatorLogos returns operator logos as 5-tuples keyed by operator ID.
func (r *ImageRepo) FindOperatorLogos(ctx context.Context, operatorIDs []int) (map[int][]any, error) {
	if len(operatorIDs) == 0 {
		return map[int][]any{}, nil
	}

	query, args, err := sqlx.In(`
		SELECT s.path, s.width, s.height, i.operator_id
		FROM images_operator_logo i
		JOIN storage s ON s.storage_id = i.storage_id
		WHERE i.operator_id IN (?)`, operatorIDs)
	if err != nil {
		return nil, fmt.Errorf("image logo IN expand: %w", err)
	}

	var rows []logoRow
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("image logo query: %w", err)
	}

	result := make(map[int][]any, len(rows))
	for _, row := range rows {
		if row.Width == 0 || row.Height == 0 {
			continue
		}
		// Only keep first logo per operator
		if _, ok := result[row.OperatorID]; ok {
			continue
		}
		result[row.OperatorID] = []any{row.Path, row.Width, row.Height, domain.ImageTypeOperatorLogo, 0}
	}
	return result, nil
}

// classImageRow is the scan target for class image queries.
type classImageRow struct {
	Path       string `db:"path"`
	Width      int    `db:"width"`
	Height     int    `db:"height"`
	OperatorID int    `db:"operator_id"`
	ClassID    int    `db:"class_id"`
	View       int    `db:"view"` // NULL→0 via COALESCE (NULL = inside, same as PHP)
}

// OperatorClassPair identifies an operator+class combination.
type OperatorClassPair struct {
	OperatorID int
	ClassID    int
}

// FindClassImages loads images from images_class table.
func (r *ImageRepo) FindClassImages(ctx context.Context, pairs []OperatorClassPair) (*domain.ImageCollection, error) {
	coll := domain.NewImageCollection()
	if len(pairs) == 0 {
		return coll, nil
	}

	var conditions []string
	var args []interface{}
	for _, p := range pairs {
		conditions = append(conditions, "(?,?)")
		args = append(args, p.OperatorID, p.ClassID)
	}

	query := fmt.Sprintf(`
		SELECT s.path, s.width, s.height, i.operator_id, i.class_id, COALESCE(i.view, 0) AS view
		FROM images_class i
		JOIN storage s ON s.storage_id = i.storage_id
		WHERE (i.operator_id, i.class_id) IN (%s)`, strings.Join(conditions, ","))

	var rows []classImageRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return coll, fmt.Errorf("class images query: %w", err)
	}

	for _, row := range rows {
		if row.Width == 0 || row.Height == 0 {
			continue
		}
		imgType := domain.ImageTypeClassOutside
		if row.View == 0 {
			imgType = domain.ImageTypeClassInside
		}
		img := domain.ImageTuple{row.Path, row.Width, row.Height, imgType, 0}
		coll.AddClassImage(row.OperatorID, row.ClassID, img)
	}
	return coll, nil
}

// customClassImageRow is the scan target for custom class image queries.
// Uses pointers for nullable DB columns to preserve PHP NULL semantics.
type customClassImageRow struct {
	Path       string  `db:"path"`
	Width      int     `db:"width"`
	Height     int     `db:"height"`
	OperatorID int     `db:"operator_id"`
	ClassID    int     `db:"class_id"`
	FromID     *int    `db:"from_id"`
	ToID       *int    `db:"to_id"`
	OfficialID *string `db:"official_id"`
	Type       string  `db:"type"`
	ImageOrder int     `db:"image_order"`
}

// LoadCustomClassImages loads custom class images into an existing ImageCollection.
// Matches PHP ImageRepository::getBatchCustomClassImages().
func (r *ImageRepo) LoadCustomClassImages(ctx context.Context, coll *domain.ImageCollection, pairs []OperatorClassPair) error {
	if len(pairs) == 0 {
		return nil
	}

	var conditions []string
	var args []interface{}
	for _, p := range pairs {
		conditions = append(conditions, "(?,?)")
		args = append(args, p.OperatorID, p.ClassID)
	}

	// No COALESCE on from_id, to_id, official_id — preserve NULL for PHP-matching bucket routing.
	query := fmt.Sprintf(`
		SELECT s.path, s.width, s.height, i.operator_id, i.class_id,
		       i.from_id, i.to_id, i.official_id,
		       COALESCE(i.type, '') AS type, COALESCE(i.image_order, 0) AS image_order
		FROM images_custom_class i
		JOIN storage s ON s.storage_id = i.storage_id
		WHERE (i.operator_id, i.class_id) IN (%s)`, strings.Join(conditions, ","))

	var rows []customClassImageRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return fmt.Errorf("custom class images query: %w", err)
	}

	for _, row := range rows {
		if row.Width == 0 || row.Height == 0 {
			continue
		}
		imgType := typeCodeToInt(row.Type)
		img := domain.ImageTuple{row.Path, row.Width, row.Height, imgType, row.ImageOrder}
		coll.AddCustomClassImage(row.OperatorID, row.ClassID, img, row.OfficialID, row.FromID, row.ToID)
	}
	return nil
}

// routeImageRow is the scan target for route image queries.
type routeImageRow struct {
	Path    string `db:"path"`
	Width   int    `db:"width"`
	Height  int    `db:"height"`
	RouteID int    `db:"route_id"`
}

// LoadRouteImages loads route images into an existing ImageCollection.
// Matches PHP ImageRepository::getBatchRouteImages() — no ORDER BY.
func (r *ImageRepo) LoadRouteImages(ctx context.Context, coll *domain.ImageCollection, routeIDs []int) error {
	if len(routeIDs) == 0 {
		return nil
	}

	query, args, err := sqlx.In(`
		SELECT s.path, s.width, s.height, i.route_id
		FROM images_route i
		JOIN storage s ON s.storage_id = i.storage_id
		WHERE i.route_id IN (?)`, routeIDs)
	if err != nil {
		return fmt.Errorf("route images IN expand: %w", err)
	}

	var rows []routeImageRow
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("route images query: %w", err)
	}

	for _, row := range rows {
		if row.Width == 0 || row.Height == 0 {
			continue
		}
		img := domain.ImageTuple{row.Path, row.Width, row.Height, domain.ImageTypeRoute, 0}
		coll.AddRouteImage(row.RouteID, img)
	}
	return nil
}

// typeCodeToInt converts the string type code from images_custom_class to an int constant.
func typeCodeToInt(code string) int {
	switch code {
	case "inside":
		return domain.ImageTypeClassInside
	case "outside":
		return domain.ImageTypeClassOutside
	case "other":
		return domain.ImageTypeClassOther
	default:
		return domain.ImageTypeUnknown
	}
}
