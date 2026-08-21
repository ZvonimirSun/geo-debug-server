package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

const (
	WebMercatorQuad       = "WebMercatorQuad"
	WorldCRS84Quad        = "WorldCRS84Quad"
	CGCS2000Quad          = "CGCS2000Quad"
	DefaultSchemeCacheTTL = 5 * time.Minute
)

var (
	ErrSchemeNotFound = errors.New("tile scheme not found")
	ErrSchemeExists   = errors.New("tile scheme already exists")
	ErrInvalidScheme  = errors.New("invalid tile scheme")
	ErrStoreClosed    = errors.New("tile scheme store is closed")
)

type Scheme struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	CRS              string        `json:"crs"`
	MetersPerUnit    float64       `json:"metersPerUnit"`
	TileWidth        int           `json:"tileWidth"`
	TileHeight       int           `json:"tileHeight"`
	MinZoom          int           `json:"minZoom"`
	MaxZoom          int           `json:"maxZoom"`
	OriginX          float64       `json:"originX"`
	OriginY          float64       `json:"originY"`
	MinX             float64       `json:"minX"`
	MinY             float64       `json:"minY"`
	MaxX             float64       `json:"maxX"`
	MaxY             float64       `json:"maxY"`
	YCoordinateFirst bool          `json:"yCoordinateFirst"`
	IsDefault        bool          `json:"isDefault"`
	Levels           []MatrixLevel `json:"levels"`
}

type MatrixLevel struct {
	Zoom         int     `json:"zoom"`
	Identifier   string  `json:"identifier"`
	Resolution   float64 `json:"resolution"`
	MatrixWidth  int64   `json:"matrixWidth"`
	MatrixHeight int64   `json:"matrixHeight"`
}

type Bounds struct {
	MinX float64
	MinY float64
	MaxX float64
	MaxY float64
}

type Store struct {
	db *sql.DB

	cacheMu     sync.RWMutex
	cache       map[string]*schemeCacheEntry
	cacheTTL    time.Duration
	schemeLoads uint64
	closed      bool
}

type schemeCacheEntry struct {
	scheme    Scheme
	expiresAt atomic.Int64
	timer     *time.Timer
}

func Open(ctx context.Context, databasePath string) (*Store, error) {
	return OpenWithCacheTTL(ctx, databasePath, DefaultSchemeCacheTTL)
}

func OpenWithCacheTTL(ctx context.Context, databasePath string, cacheTTL time.Duration) (*Store, error) {
	initialize, err := shouldInitializeDatabase(databasePath)
	if err != nil {
		return nil, err
	}
	dsn, err := databaseDSN(databasePath)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db, cache: make(map[string]*schemeCacheEntry), cacheTTL: cacheTTL}
	if initialize {
		err = s.init(ctx)
	} else {
		err = db.PingContext(ctx)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func shouldInitializeDatabase(databasePath string) (bool, error) {
	path := databasePath
	if strings.HasPrefix(path, "file:") {
		path = strings.TrimPrefix(path, "file:")
		if index := strings.IndexByte(path, '?'); index >= 0 {
			query, err := url.ParseQuery(path[index+1:])
			if err != nil {
				return false, fmt.Errorf("parse database URI query: %w", err)
			}
			if strings.EqualFold(query.Get("mode"), "memory") {
				return true, nil
			}
			path = path[:index]
		}
		if path == ":memory:" {
			return true, nil
		}
		decoded, err := url.PathUnescape(path)
		if err != nil {
			return false, fmt.Errorf("decode database URI path: %w", err)
		}
		path = filepath.FromSlash(decoded)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("resolve database path: %w", err)
	}
	_, err = os.Stat(absPath)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	return false, fmt.Errorf("stat database file: %w", err)
}

func databaseDSN(databasePath string) (string, error) {
	if strings.HasPrefix(databasePath, "file:") {
		separator := "?"
		if strings.Contains(databasePath, "?") {
			separator = "&"
		}
		return databasePath + separator + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", nil
	}
	absPath, err := filepath.Abs(databasePath)
	if err != nil {
		return "", fmt.Errorf("resolve database path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", fmt.Errorf("create database directory: %w", err)
	}
	return "file:" + filepath.ToSlash(absPath) + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", nil
}

func (s *Store) Close() error {
	s.cacheMu.Lock()
	if s.closed {
		s.cacheMu.Unlock()
		return nil
	}
	s.closed = true
	for key, entry := range s.cache {
		if entry.timer != nil {
			entry.timer.Stop()
		}
		delete(s.cache, key)
	}
	s.cacheMu.Unlock()
	return s.db.Close()
}

func (s *Store) init(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin database initialization: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("create database schema: %w", err)
	}
	for _, scheme := range defaultSchemes() {
		if err := validateScheme(scheme); err != nil {
			return err
		}
		if err := createSchemeTx(ctx, tx, scheme); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit database initialization: %w", err)
	}
	return nil
}

func (s *Store) Scheme(ctx context.Context, id string) (Scheme, error) {
	key := schemeCacheKey(id)
	if scheme, ok, err := s.cachedScheme(key); err != nil {
		return Scheme{}, err
	} else if ok {
		return scheme, nil
	}

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.closed {
		return Scheme{}, ErrStoreClosed
	}
	if scheme, ok := s.cachedSchemeLocked(key, time.Now()); ok {
		return cloneScheme(scheme), nil
	}
	scheme, err := s.loadScheme(ctx, id)
	if err != nil {
		return Scheme{}, err
	}
	s.schemeLoads++
	s.cacheSchemeLocked(key, scheme)
	return cloneScheme(scheme), nil
}

func (s *Store) loadScheme(ctx context.Context, id string) (Scheme, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, crs, meters_per_unit, tile_width, tile_height, min_zoom, max_zoom,
		       origin_x, origin_y, min_x, min_y, max_x, max_y, y_coordinate_first, is_default
		FROM tile_schemes WHERE lower(id) = lower(?)`, id)
	var scheme Scheme
	if err := row.Scan(
		&scheme.ID, &scheme.Name, &scheme.CRS, &scheme.MetersPerUnit, &scheme.TileWidth, &scheme.TileHeight,
		&scheme.MinZoom, &scheme.MaxZoom, &scheme.OriginX, &scheme.OriginY,
		&scheme.MinX, &scheme.MinY, &scheme.MaxX, &scheme.MaxY,
		&scheme.YCoordinateFirst, &scheme.IsDefault,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Scheme{}, ErrSchemeNotFound
		}
		return Scheme{}, fmt.Errorf("query tile scheme: %w", err)
	}
	levels, err := s.levels(ctx, scheme.ID)
	if err != nil {
		return Scheme{}, err
	}
	scheme.Levels = levels
	return scheme, nil
}

func (s *Store) DefaultScheme(ctx context.Context) (Scheme, error) {
	const defaultCacheKey = "\x00default"
	if scheme, ok, err := s.cachedScheme(defaultCacheKey); err != nil {
		return Scheme{}, err
	} else if ok {
		return scheme, nil
	}

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.closed {
		return Scheme{}, ErrStoreClosed
	}
	if scheme, ok := s.cachedSchemeLocked(defaultCacheKey, time.Now()); ok {
		return cloneScheme(scheme), nil
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id FROM tile_schemes ORDER BY is_default DESC, id LIMIT 1`)
	var id string
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Scheme{}, ErrSchemeNotFound
		}
		return Scheme{}, fmt.Errorf("query default tile scheme: %w", err)
	}
	key := schemeCacheKey(id)
	scheme, ok := s.cachedSchemeLocked(key, time.Now())
	if !ok {
		loaded, err := s.loadScheme(ctx, id)
		if err != nil {
			return Scheme{}, err
		}
		scheme = loaded
		s.schemeLoads++
		s.cacheSchemeLocked(key, scheme)
	}
	s.cacheSchemeLocked(defaultCacheKey, scheme)
	return cloneScheme(scheme), nil
}

func (s *Store) Schemes(ctx context.Context) ([]Scheme, error) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.closed {
		return nil, ErrStoreClosed
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM tile_schemes ORDER BY is_default DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("query tile schemes: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan tile scheme: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tile schemes: %w", err)
	}
	result := make([]Scheme, 0, len(ids))
	for _, id := range ids {
		key := schemeCacheKey(id)
		scheme, ok := s.cachedSchemeLocked(key, time.Now())
		if !ok {
			scheme, err = s.loadScheme(ctx, id)
			if err != nil {
				return nil, err
			}
			s.schemeLoads++
			s.cacheSchemeLocked(key, scheme)
		}
		result = append(result, cloneScheme(scheme))
	}
	return result, nil
}

func (s *Store) levels(ctx context.Context, schemeID string) ([]MatrixLevel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT zoom, identifier, resolution, matrix_width, matrix_height
		FROM tile_matrix_levels WHERE scheme_id = ? ORDER BY zoom`, schemeID)
	if err != nil {
		return nil, fmt.Errorf("query tile matrix levels: %w", err)
	}
	defer rows.Close()
	var levels []MatrixLevel
	for rows.Next() {
		var level MatrixLevel
		if err := rows.Scan(&level.Zoom, &level.Identifier, &level.Resolution,
			&level.MatrixWidth, &level.MatrixHeight); err != nil {
			return nil, fmt.Errorf("scan tile matrix level: %w", err)
		}
		levels = append(levels, level)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tile matrix levels: %w", err)
	}
	return levels, nil
}

func schemeCacheKey(id string) string {
	return strings.ToLower(id)
}

func (s *Store) cachedScheme(key string) (Scheme, bool, error) {
	now := time.Now()
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	if s.closed {
		return Scheme{}, false, ErrStoreClosed
	}
	if s.cacheTTL <= 0 {
		return Scheme{}, false, nil
	}
	entry := s.cache[key]
	if entry == nil || now.UnixNano() >= entry.expiresAt.Load() {
		return Scheme{}, false, nil
	}
	extendSchemeExpiry(entry, now.Add(s.cacheTTL).UnixNano())
	return cloneScheme(entry.scheme), true, nil
}

func extendSchemeExpiry(entry *schemeCacheEntry, expiresAt int64) {
	for {
		current := entry.expiresAt.Load()
		if current >= expiresAt || entry.expiresAt.CompareAndSwap(current, expiresAt) {
			return
		}
	}
}

func (s *Store) cachedSchemeLocked(key string, now time.Time) (Scheme, bool) {
	if s.cacheTTL <= 0 {
		return Scheme{}, false
	}
	entry, ok := s.cache[key]
	if !ok {
		return Scheme{}, false
	}
	if now.UnixNano() >= entry.expiresAt.Load() {
		if entry.timer != nil {
			entry.timer.Stop()
		}
		delete(s.cache, key)
		return Scheme{}, false
	}
	entry.expiresAt.Store(now.Add(s.cacheTTL).UnixNano())
	return entry.scheme, true
}

func (s *Store) cacheSchemeLocked(key string, scheme Scheme) {
	if s.cacheTTL <= 0 || s.closed {
		return
	}
	if existing := s.cache[key]; existing != nil && existing.timer != nil {
		existing.timer.Stop()
	}
	entry := &schemeCacheEntry{scheme: cloneScheme(scheme)}
	entry.expiresAt.Store(time.Now().Add(s.cacheTTL).UnixNano())
	s.cache[key] = entry
	entry.timer = time.AfterFunc(s.cacheTTL, func() {
		s.expireScheme(key, entry)
	})
}

func (s *Store) expireScheme(key string, expected *schemeCacheEntry) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.closed {
		return
	}
	entry := s.cache[key]
	if entry == nil || entry != expected {
		return
	}
	remaining := time.Until(time.Unix(0, entry.expiresAt.Load()))
	if remaining > 0 {
		entry.timer.Reset(remaining)
		return
	}
	delete(s.cache, key)
}

func cloneScheme(scheme Scheme) Scheme {
	scheme.Levels = append([]MatrixLevel(nil), scheme.Levels...)
	return scheme
}

func (s Scheme) Level(identifier string) (MatrixLevel, bool) {
	for _, level := range s.Levels {
		if level.Identifier == identifier {
			return level, true
		}
	}
	return MatrixLevel{}, false
}

func (s Scheme) TileBounds(level MatrixLevel, column, row int64) Bounds {
	tileSpanX := float64(s.TileWidth) * level.Resolution
	tileSpanY := float64(s.TileHeight) * level.Resolution
	minX := s.OriginX + float64(column)*tileSpanX
	maxY := s.OriginY - float64(row)*tileSpanY
	return Bounds{MinX: minX, MinY: maxY - tileSpanY, MaxX: minX + tileSpanX, MaxY: maxY}
}

func defaultSchemes() []Scheme {
	const (
		mercatorHalfWorld = 20037508.342789244
		metersPerDegree   = 111319.49079327358
	)
	mercator := Scheme{
		ID: WebMercatorQuad, Name: "Web Mercator Quad", CRS: "EPSG:3857",
		MetersPerUnit: 1,
		TileWidth:     256, TileHeight: 256, MinZoom: 0, MaxZoom: 22,
		OriginX: -mercatorHalfWorld, OriginY: mercatorHalfWorld,
		MinX: -mercatorHalfWorld, MinY: -mercatorHalfWorld,
		MaxX: mercatorHalfWorld, MaxY: mercatorHalfWorld,
	}
	crs84 := Scheme{
		ID: WorldCRS84Quad, Name: "World CRS84 Quad", CRS: "CRS:84",
		MetersPerUnit: metersPerDegree,
		TileWidth:     256, TileHeight: 256, MinZoom: 0, MaxZoom: 23,
		OriginX: -180, OriginY: 90, MinX: -180, MinY: -90, MaxX: 180, MaxY: 90,
	}
	cgcs2000 := Scheme{
		ID: CGCS2000Quad, Name: "CGCS2000 Quad", CRS: "EPSG:4490",
		MetersPerUnit: metersPerDegree,
		TileWidth:     256, TileHeight: 256, MinZoom: 0, MaxZoom: 23,
		OriginX: -180, OriginY: 90, MinX: -180, MinY: -90, MaxX: 180, MaxY: 90,
		YCoordinateFirst: true, IsDefault: true,
	}
	for zoom := 0; zoom <= 22; zoom++ {
		factor := math.Exp2(float64(zoom))
		mercator.Levels = append(mercator.Levels, MatrixLevel{
			Zoom: zoom, Identifier: fmt.Sprint(zoom),
			Resolution:  156543.03392804097 / factor,
			MatrixWidth: int64(factor), MatrixHeight: int64(factor),
		})
	}
	for zoom := 0; zoom <= 23; zoom++ {
		factor := math.Exp2(float64(zoom))
		matrixHeight := int64(1)
		if zoom > 0 {
			matrixHeight = int64(factor / 2)
		}
		level := MatrixLevel{
			Zoom: zoom, Identifier: fmt.Sprint(zoom),
			Resolution:  1.40625 / factor,
			MatrixWidth: int64(factor), MatrixHeight: matrixHeight,
		}
		crs84.Levels = append(crs84.Levels, level)
		cgcs2000.Levels = append(cgcs2000.Levels, level)
	}
	return []Scheme{mercator, crs84, cgcs2000}
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS tile_schemes (
	    id TEXT PRIMARY KEY,
	    name TEXT NOT NULL,
	    crs TEXT NOT NULL,
	    meters_per_unit REAL NOT NULL CHECK(meters_per_unit > 0),
    tile_width INTEGER NOT NULL CHECK(tile_width > 0),
    tile_height INTEGER NOT NULL CHECK(tile_height > 0),
    min_zoom INTEGER NOT NULL,
    max_zoom INTEGER NOT NULL CHECK(max_zoom >= min_zoom),
    origin_x REAL NOT NULL,
    origin_y REAL NOT NULL,
    min_x REAL NOT NULL,
    min_y REAL NOT NULL,
    max_x REAL NOT NULL,
    max_y REAL NOT NULL,
    y_coordinate_first INTEGER NOT NULL DEFAULT 0 CHECK(y_coordinate_first IN (0, 1)),
    is_default INTEGER NOT NULL DEFAULT 0 CHECK(is_default IN (0, 1))
);

CREATE TABLE IF NOT EXISTS tile_matrix_levels (
    scheme_id TEXT NOT NULL REFERENCES tile_schemes(id) ON DELETE CASCADE,
	    zoom INTEGER NOT NULL,
	    identifier TEXT NOT NULL,
	    resolution REAL NOT NULL CHECK(resolution > 0),
    matrix_width INTEGER NOT NULL CHECK(matrix_width > 0),
    matrix_height INTEGER NOT NULL CHECK(matrix_height > 0),
    PRIMARY KEY(scheme_id, zoom),
    UNIQUE(scheme_id, identifier)
);

CREATE INDEX IF NOT EXISTS idx_tile_matrix_levels_scheme
ON tile_matrix_levels(scheme_id, zoom);
`
