package repository

import (
	"context"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// TripPoolSet represents a multi-leg trip set from trip_pool4_set.
type TripPoolSet struct {
	SetID             int     `db:"set_id"`
	TripKey           string  `db:"trip_key"`
	Trip1Key          string  `db:"trip1_key"`
	Trip2Key          string  `db:"trip2_key"`
	Trip3Key          *string `db:"trip3_key"`
	PackID            int     `db:"pack_id"`
	Transit1Guarantee int     `db:"transit1_guarantee"`
	Transit2Guarantee int     `db:"transit2_guarantee"`
	Trip2Day          int     `db:"trip2_day"`
	Trip3Day          int     `db:"trip3_day"`
}

// TripPoolSetRepo handles queries on the trip_pool4_set table.
type TripPoolSetRepo struct {
	connMgr *ConnectionManagerRef

	// In-memory cache per region (enabled via CACHE_SETS=true)
	cacheEnabled bool
	cacheTTL     time.Duration
	logger       *zap.Logger
	mu           sync.RWMutex
	byRegion     map[string]*regionSetCache
}

type regionSetCache struct {
	sets     map[int]TripPoolSet
	loadedAt time.Time
}

type ConnectionManagerRef struct {
	getDB func(region string) *sqlx.DB
}

func NewTripPoolSetRepo(getDB func(region string) *sqlx.DB) *TripPoolSetRepo {
	return &TripPoolSetRepo{
		connMgr:  &ConnectionManagerRef{getDB: getDB},
		byRegion: make(map[string]*regionSetCache),
	}
}

// EnableCache turns on in-memory caching with the given TTL.
func (r *TripPoolSetRepo) EnableCache(ttl time.Duration, logger *zap.Logger) {
	r.cacheEnabled = true
	r.cacheTTL = ttl
	r.logger = logger
}

// PreloadRegion loads all sets for a region into cache during startup.
func (r *TripPoolSetRepo) PreloadRegion(ctx context.Context, region string) {
	if err := r.loadRegion(ctx, region); err != nil {
		if r.logger != nil {
			r.logger.Warn("set cache: preload failed", zap.String("region", region), zap.Error(err))
		}
	}
}

// FindBySetIDs returns trip sets by their set IDs.
func (r *TripPoolSetRepo) FindBySetIDs(ctx context.Context, region string, setIDs []int) ([]TripPoolSet, error) {
	if len(setIDs) == 0 {
		return nil, nil
	}

	// Try cache first
	if r.cacheEnabled {
		if sets, ok := r.fromCache(region, setIDs); ok {
			return sets, nil
		}
		// Cache miss or stale — load all sets for this region
		if err := r.loadRegion(ctx, region); err != nil {
			// Fall through to direct query on load failure
			if r.logger != nil {
				r.logger.Warn("set cache: failed to load region, falling back to DB",
					zap.String("region", region), zap.Error(err))
			}
		} else {
			if sets, ok := r.fromCache(region, setIDs); ok {
				return sets, nil
			}
		}
	}

	// Direct DB query (fallback or cache disabled)
	return r.queryBySetIDs(ctx, region, setIDs)
}

func (r *TripPoolSetRepo) fromCache(region string, setIDs []int) ([]TripPoolSet, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rc, ok := r.byRegion[region]
	if !ok || time.Since(rc.loadedAt) > r.cacheTTL {
		return nil, false
	}
	result := make([]TripPoolSet, 0, len(setIDs))
	for _, id := range setIDs {
		if set, ok := rc.sets[id]; ok {
			result = append(result, set)
		}
	}
	return result, true
}

func (r *TripPoolSetRepo) loadRegion(ctx context.Context, region string) error {
	db := r.connMgr.getDB(region)
	var allSets []TripPoolSet
	err := db.SelectContext(ctx, &allSets,
		`SELECT set_id, trip_key, trip1_key, trip2_key, trip3_key,
		        COALESCE(pack_id, 0) AS pack_id,
		        COALESCE(transit1_guarantee, 0) AS transit1_guarantee,
		        COALESCE(transit2_guarantee, 0) AS transit2_guarantee,
		        COALESCE(trip2_day, 0) AS trip2_day,
		        COALESCE(trip3_day, 0) AS trip3_day
		 FROM trip_pool4_set`)
	if err != nil {
		return err
	}

	setMap := make(map[int]TripPoolSet, len(allSets))
	for _, s := range allSets {
		setMap[s.SetID] = s
	}

	r.mu.Lock()
	r.byRegion[region] = &regionSetCache{
		sets:     setMap,
		loadedAt: time.Now(),
	}
	r.mu.Unlock()

	if r.logger != nil {
		r.logger.Info("set cache: loaded region",
			zap.String("region", region), zap.Int("sets", len(setMap)))
	}
	return nil
}

func (r *TripPoolSetRepo) queryBySetIDs(ctx context.Context, region string, setIDs []int) ([]TripPoolSet, error) {
	db := r.connMgr.getDB(region)
	query, args, err := sqlx.In(
		`SELECT set_id, trip_key, trip1_key, trip2_key, trip3_key,
		        COALESCE(pack_id, 0) AS pack_id,
		        COALESCE(transit1_guarantee, 0) AS transit1_guarantee,
		        COALESCE(transit2_guarantee, 0) AS transit2_guarantee,
		        COALESCE(trip2_day, 0) AS trip2_day,
		        COALESCE(trip3_day, 0) AS trip3_day
		 FROM trip_pool4_set WHERE set_id IN (?)`, setIDs)
	if err != nil {
		return nil, err
	}
	var sets []TripPoolSet
	err = db.SelectContext(ctx, &sets, db.Rebind(query), args...)
	return sets, err
}
