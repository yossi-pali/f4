package refcache

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/repository"
)

// CacheConfig controls which entity caches are enabled.
type CacheConfig struct {
	EnableOperators    bool          // CACHE_OPERATORS=true (default false)
	EnableStations     bool          // CACHE_STATIONS=true
	EnableClasses      bool          // CACHE_CLASSES=true
	EnableReasons      bool          // CACHE_REASONS=true
	EnableIntegration  bool          // CACHE_INTEGRATION=true
	RefreshTTL         time.Duration // CACHE_REFRESH_TTL=5m (default 5min)
}

// RefDataCache provides in-memory caching of reference data.
// Each entity type is independently toggleable. When disabled, callers
// fall back to the DB query.
type RefDataCache struct {
	cfg    CacheConfig
	db     *sqlx.DB
	logger *zap.Logger
	stopCh chan struct{}

	mu                  sync.RWMutex
	operators           map[int]domain.Operator
	operatorRatings     map[int]repository.OperatorRating
	stations            map[int]domain.Station
	classes             map[int]domain.VehicleClass
	manualIntegrationID int
	lastRefresh         time.Time
}

// New creates a RefDataCache. Call Start() to begin background refresh.
func New(cfg CacheConfig, db *sqlx.DB, logger *zap.Logger) *RefDataCache {
	if cfg.RefreshTTL == 0 {
		cfg.RefreshTTL = 5 * time.Minute
	}
	return &RefDataCache{
		cfg:    cfg,
		db:     db,
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// Start loads enabled entities and begins background refresh.
func (c *RefDataCache) Start(ctx context.Context) {
	c.refresh(ctx)
	go c.backgroundRefresh()
}

// Stop halts the background refresh goroutine.
func (c *RefDataCache) Stop() {
	close(c.stopCh)
}

func (c *RefDataCache) backgroundRefresh() {
	ticker := time.NewTicker(c.cfg.RefreshTTL)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.refresh(context.Background())
		case <-c.stopCh:
			return
		}
	}
}

func (c *RefDataCache) refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if c.cfg.EnableOperators {
		ops, err := c.loadAllOperators(ctx)
		if err != nil {
			c.logger.Warn("refcache: failed to refresh operators", zap.Error(err))
		} else {
			ratings, err := c.loadAllRatings(ctx)
			if err != nil {
				c.logger.Warn("refcache: failed to refresh ratings", zap.Error(err))
				ratings = map[int]repository.OperatorRating{}
			}
			c.mu.Lock()
			c.operators = ops
			c.operatorRatings = ratings
			c.mu.Unlock()
			c.logger.Info("refcache: refreshed operators", zap.Int("count", len(ops)))
		}
	}

	if c.cfg.EnableStations {
		stations, err := c.loadAllStations(ctx)
		if err != nil {
			c.logger.Warn("refcache: failed to refresh stations", zap.Error(err))
		} else {
			c.mu.Lock()
			c.stations = stations
			c.mu.Unlock()
			c.logger.Info("refcache: refreshed stations", zap.Int("count", len(stations)))
		}
	}

	if c.cfg.EnableClasses {
		classes, err := c.loadAllClasses(ctx)
		if err != nil {
			c.logger.Warn("refcache: failed to refresh classes", zap.Error(err))
		} else {
			c.mu.Lock()
			c.classes = classes
			c.mu.Unlock()
			c.logger.Info("refcache: refreshed classes", zap.Int("count", len(classes)))
		}
	}

	if c.cfg.EnableIntegration {
		id, err := c.loadManualIntegrationID(ctx)
		if err != nil {
			c.logger.Warn("refcache: failed to refresh integration", zap.Error(err))
		} else {
			c.mu.Lock()
			c.manualIntegrationID = id
			c.mu.Unlock()
		}
	}

	c.mu.Lock()
	c.lastRefresh = time.Now()
	c.mu.Unlock()
}

// GetOperators returns cached operators if the cache is enabled and loaded.
// Returns (data, true) on cache hit; (nil, false) on miss (caller should fall back to DB).
func (c *RefDataCache) GetOperators(ids []int) (map[int]domain.Operator, bool) {
	if !c.cfg.EnableOperators {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.operators == nil {
		return nil, false
	}
	result := make(map[int]domain.Operator, len(ids))
	for _, id := range ids {
		if op, ok := c.operators[id]; ok {
			result[id] = op
		}
	}
	return result, true
}

// GetOperatorRatings returns cached ratings if enabled.
func (c *RefDataCache) GetOperatorRatings(ids []int) (map[int]repository.OperatorRating, bool) {
	if !c.cfg.EnableOperators {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.operatorRatings == nil {
		return nil, false
	}
	result := make(map[int]repository.OperatorRating, len(ids))
	for _, id := range ids {
		if r, ok := c.operatorRatings[id]; ok {
			result[id] = r
		}
	}
	return result, true
}

// GetStations returns cached stations if the cache is enabled.
func (c *RefDataCache) GetStations(ids []int) (map[int]domain.Station, bool) {
	if !c.cfg.EnableStations {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.stations == nil {
		return nil, false
	}
	result := make(map[int]domain.Station, len(ids))
	for _, id := range ids {
		if st, ok := c.stations[id]; ok {
			result[id] = st
		}
	}
	return result, true
}

// GetClasses returns cached classes if the cache is enabled.
func (c *RefDataCache) GetClasses(ids []int) (map[int]domain.VehicleClass, bool) {
	if !c.cfg.EnableClasses {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.classes == nil {
		return nil, false
	}
	result := make(map[int]domain.VehicleClass, len(ids))
	for _, id := range ids {
		if cl, ok := c.classes[id]; ok {
			result[id] = cl
		}
	}
	return result, true
}

// GetManualIntegrationID returns cached integration ID if enabled.
func (c *RefDataCache) GetManualIntegrationID() (int, bool) {
	if !c.cfg.EnableIntegration {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.manualIntegrationID, c.manualIntegrationID > 0
}

// --- DB loading functions ---

func (c *RefDataCache) loadAllOperators(ctx context.Context) (map[int]domain.Operator, error) {
	type row struct {
		OperatorID    int     `db:"operator_id"`
		Name          string  `db:"operator_name"`
		SellerID      int     `db:"seller_id"`
		MasterID      int     `db:"master_id"`
		Bookable      int     `db:"bookable"`
		Code          *string `db:"operator_code"`
		CounterpartID int     `db:"counterpart_id"`
	}
	var rows []row
	err := c.db.SelectContext(ctx, &rows, `
		SELECT o.operator_id, COALESCE(o.operator_name, '') AS operator_name,
		       COALESCE(o.seller_id, 0) AS seller_id,
		       COALESCE(o.master_id, 0) AS master_id, COALESCE(o.bookable, 0) AS bookable,
		       o.operator_code,
		       COALESCE(s.counterpart_id, 0) AS counterpart_id
		FROM operator o
		LEFT JOIN seller s ON s.seller_id = o.seller_id`)
	if err != nil {
		return nil, err
	}
	result := make(map[int]domain.Operator, len(rows))
	for _, r := range rows {
		result[r.OperatorID] = domain.Operator{
			OperatorID:    r.OperatorID,
			Name:          r.Name,
			Slug:          slugifyName(r.Name),
			SellerID:      r.SellerID,
			MasterID:      r.MasterID,
			Bookable:      r.Bookable != 0,
			Code:          r.Code,
			CounterpartID: r.CounterpartID,
		}
	}
	return result, nil
}

func (c *RefDataCache) loadAllRatings(ctx context.Context) (map[int]repository.OperatorRating, error) {
	c1 := "CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.data, '$.ratingAll1')), '0') AS DECIMAL(20,4))"
	c2 := "CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.data, '$.ratingAll2')), '0') AS DECIMAL(20,4))"
	c3 := "CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.data, '$.ratingAll3')), '0') AS DECIMAL(20,4))"
	c4 := "CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.data, '$.ratingAll4')), '0') AS DECIMAL(20,4))"
	c5 := "CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.data, '$.ratingAll5')), '0') AS DECIMAL(20,4))"

	weighted := "(" + c1 + "*1 + " + c2 + "*2 + " + c3 + "*3 + " + c4 + "*4 + " + c5 + "*5)"
	total := "(" + c1 + " + " + c2 + " + " + c3 + " + " + c4 + " + " + c5 + ")"

	query := `SELECT o.operator_id,
		       COALESCE(ROUND(` + weighted + ` / NULLIF(` + total + `, 0), 2), 0) AS rating,
		       CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.data, '$.ratingsCount')), '0') AS UNSIGNED) AS ratings_count
		FROM operator_extra o
		WHERE o.extra_type = 'rating'`

	var rows []repository.OperatorRating
	if err := c.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, err
	}
	result := make(map[int]repository.OperatorRating, len(rows))
	for _, r := range rows {
		result[r.OperatorID] = r
	}
	return result, nil
}

func (c *RefDataCache) loadAllStations(ctx context.Context) (map[int]domain.Station, error) {
	type row struct {
		domain.Station
		ProvinceName string `db:"province_name"`
	}
	var rows []row
	err := c.db.SelectContext(ctx, &rows, `
		SELECT s.station_id, COALESCE(s.station_name, '') AS station_name,
		       s.station_code,
		       COALESCE(s.lat, 0) AS lat, COALESCE(s.lng, 0) AS lng,
		       s.province_id, COALESCE(p.country_id, '') AS country_id,
		       COALESCE(s.vehclass_id, '') AS vehclass_id, p.timezone_id,
		       tz.timezone_name, COALESCE(s.weight_from, 0) AS weight_from,
		       COALESCE(s.coordinates_accurate, 0) AS coordinates_accurate,
		       COALESCE(p.province_name, '') AS province_name
		FROM station s
		JOIN province p ON p.province_id = s.province_id
		JOIN timezone tz ON tz.timezone_id = p.timezone_id`)
	if err != nil {
		return nil, err
	}
	result := make(map[int]domain.Station, len(rows))
	for _, r := range rows {
		st := r.Station
		st.StationSlug = slugifyName(st.StationName)
		code := ""
		if st.StationCode != nil {
			code = *st.StationCode
		}
		st.StationNameFull = buildStationNameFull(st.StationName, r.ProvinceName, st.VehclassID, code)
		result[st.StationID] = st
	}
	return result, nil
}

func (c *RefDataCache) loadAllClasses(ctx context.Context) (map[int]domain.VehicleClass, error) {
	var classes []domain.VehicleClass
	err := c.db.SelectContext(ctx, &classes, `
		SELECT class_id, COALESCE(class_name, '') AS class_name,
		       COALESCE(vehclasses, '') AS vehclasses,
		       COALESCE(is_multi_pax, 0) AS is_multi_pax,
		       COALESCE(seats, 0) AS seats
		FROM class`)
	if err != nil {
		return nil, err
	}
	result := make(map[int]domain.VehicleClass, len(classes))
	for _, cl := range classes {
		result[cl.ClassID] = cl
	}
	return result, nil
}

func (c *RefDataCache) loadManualIntegrationID(ctx context.Context) (int, error) {
	var id int
	err := c.db.GetContext(ctx, &id,
		`SELECT integration_id FROM integration WHERE integration_code = 'manual' LIMIT 1`)
	return id, err
}

// --- helpers ---

func slugifyName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
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

func buildStationNameFull(stationName, provinceName, vehclassID, stationCode string) string {
	name := stationName
	if provinceName != "" && !strings.Contains(stationName, provinceName) {
		name = name + ", " + provinceName
	}
	if vehclassID == "avia" && stationCode != "" {
		name = stationCode + " " + name
	}
	return name
}
