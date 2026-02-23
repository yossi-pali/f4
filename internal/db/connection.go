package db

import (
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"github.com/12go/f4/internal/config"
)

// ConnectionManager manages multiple MySQL connections with regional sharding.
type ConnectionManager struct {
	defaultDB *sqlx.DB
	tripPools map[string]*sqlx.DB // region code → *sqlx.DB
}

// NewConnectionManager creates connections from config.
func NewConnectionManager(cfg config.DatabaseConfig) (*ConnectionManager, error) {
	cm := &ConnectionManager{
		tripPools: make(map[string]*sqlx.DB),
	}

	if cfg.Default != "" {
		db, err := openDB(cfg.Default)
		if err != nil {
			return nil, fmt.Errorf("default DB: %w", err)
		}
		cm.defaultDB = db
	}

	for region, dsn := range cfg.TripPool {
		if dsn == "" {
			continue
		}
		db, err := openDB(dsn)
		if err != nil {
			return nil, fmt.Errorf("trip_pool region %s: %w", region, err)
		}
		cm.tripPools[region] = db
	}

	return cm, nil
}

func openDB(dsn string) (*sqlx.DB, error) {
	db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping failed: %w", err)
	}
	return db, nil
}

// Default returns the default (main metadata) database connection.
func (cm *ConnectionManager) Default() *sqlx.DB {
	return cm.defaultDB
}

// TripPool returns the regional trip pool database for the given region code.
func (cm *ConnectionManager) TripPool(region string) *sqlx.DB {
	if db, ok := cm.tripPools[region]; ok {
		return db
	}
	// Fallback to first available pool
	for _, db := range cm.tripPools {
		return db
	}
	return cm.defaultDB
}

// Close closes all database connections.
func (cm *ConnectionManager) Close() error {
	if cm.defaultDB != nil {
		cm.defaultDB.Close()
	}
	for _, db := range cm.tripPools {
		db.Close()
	}
	return nil
}
