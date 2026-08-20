package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	WebMercatorQuad = "WebMercatorQuad"
	WorldCRS84Quad  = "WorldCRS84Quad"
)

var ErrSchemeNotFound = errors.New("tile scheme not found")

type Scheme struct {
	ID         string
	Name       string
	CRS        string
	TileWidth  int
	TileHeight int
	MinZoom    int
	MaxZoom    int
	OriginX    float64
	OriginY    float64
	MinX       float64
	MinY       float64
	MaxX       float64
	MaxY       float64
	IsDefault  bool
	Levels     []MatrixLevel
}

type MatrixLevel struct {
	Zoom             int
	Identifier       string
	Resolution       float64
	ScaleDenominator float64
	MatrixWidth      int64
	MatrixHeight     int64
}

type Bounds struct {
	MinX float64
	MinY float64
	MaxX float64
	MaxY float64
}

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, databasePath string) (*Store, error) {
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
	s := &Store{db: db}
	if err := s.init(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
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

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) init(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin database initialization: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("create database schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO schema_version(version, applied_at) VALUES(1, ?)`,
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}
	for _, scheme := range defaultSchemes() {
		if err := seedScheme(ctx, tx, scheme); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit database initialization: %w", err)
	}
	return nil
}

func seedScheme(ctx context.Context, tx *sql.Tx, scheme Scheme) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO tile_schemes(
			id, name, crs, tile_width, tile_height, min_zoom, max_zoom,
			origin_x, origin_y, min_x, min_y, max_x, max_y, is_default
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING`,
		scheme.ID, scheme.Name, scheme.CRS, scheme.TileWidth, scheme.TileHeight,
		scheme.MinZoom, scheme.MaxZoom, scheme.OriginX, scheme.OriginY,
		scheme.MinX, scheme.MinY, scheme.MaxX, scheme.MaxY, scheme.IsDefault)
	if err != nil {
		return fmt.Errorf("seed tile scheme %s: %w", scheme.ID, err)
	}
	for _, level := range scheme.Levels {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tile_matrix_levels(
				scheme_id, zoom, identifier, resolution, scale_denominator, matrix_width, matrix_height
			) VALUES(?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT DO NOTHING`,
			scheme.ID, level.Zoom, level.Identifier, level.Resolution,
			level.ScaleDenominator, level.MatrixWidth, level.MatrixHeight); err != nil {
			return fmt.Errorf("seed tile matrix %s/%d: %w", scheme.ID, level.Zoom, err)
		}
	}
	return nil
}

func (s *Store) Scheme(ctx context.Context, id string) (Scheme, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, crs, tile_width, tile_height, min_zoom, max_zoom,
		       origin_x, origin_y, min_x, min_y, max_x, max_y, is_default
		FROM tile_schemes WHERE lower(id) = lower(?)`, id)
	var scheme Scheme
	if err := row.Scan(
		&scheme.ID, &scheme.Name, &scheme.CRS, &scheme.TileWidth, &scheme.TileHeight,
		&scheme.MinZoom, &scheme.MaxZoom, &scheme.OriginX, &scheme.OriginY,
		&scheme.MinX, &scheme.MinY, &scheme.MaxX, &scheme.MaxY, &scheme.IsDefault,
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
	row := s.db.QueryRowContext(ctx, `
		SELECT id FROM tile_schemes ORDER BY is_default DESC, id LIMIT 1`)
	var id string
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Scheme{}, ErrSchemeNotFound
		}
		return Scheme{}, fmt.Errorf("query default tile scheme: %w", err)
	}
	return s.Scheme(ctx, id)
}

func (s *Store) Schemes(ctx context.Context) ([]Scheme, error) {
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
		scheme, err := s.Scheme(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, scheme)
	}
	return result, nil
}

func (s *Store) levels(ctx context.Context, schemeID string) ([]MatrixLevel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT zoom, identifier, resolution, scale_denominator, matrix_width, matrix_height
		FROM tile_matrix_levels WHERE scheme_id = ? ORDER BY zoom`, schemeID)
	if err != nil {
		return nil, fmt.Errorf("query tile matrix levels: %w", err)
	}
	defer rows.Close()
	var levels []MatrixLevel
	for rows.Next() {
		var level MatrixLevel
		if err := rows.Scan(&level.Zoom, &level.Identifier, &level.Resolution,
			&level.ScaleDenominator, &level.MatrixWidth, &level.MatrixHeight); err != nil {
			return nil, fmt.Errorf("scan tile matrix level: %w", err)
		}
		levels = append(levels, level)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tile matrix levels: %w", err)
	}
	return levels, nil
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
	const mercatorHalfWorld = 20037508.342789244
	mercator := Scheme{
		ID: WebMercatorQuad, Name: "Web Mercator Quad", CRS: "EPSG:3857",
		TileWidth: 256, TileHeight: 256, MinZoom: 0, MaxZoom: 22,
		OriginX: -mercatorHalfWorld, OriginY: mercatorHalfWorld,
		MinX: -mercatorHalfWorld, MinY: -mercatorHalfWorld,
		MaxX: mercatorHalfWorld, MaxY: mercatorHalfWorld, IsDefault: true,
	}
	crs84 := Scheme{
		ID: WorldCRS84Quad, Name: "World CRS84 Quad", CRS: "CRS:84",
		TileWidth: 256, TileHeight: 256, MinZoom: 0, MaxZoom: 22,
		OriginX: -180, OriginY: 90, MinX: -180, MinY: -90, MaxX: 180, MaxY: 90,
	}
	for zoom := 0; zoom <= 22; zoom++ {
		factor := math.Exp2(float64(zoom))
		mercator.Levels = append(mercator.Levels, MatrixLevel{
			Zoom: zoom, Identifier: fmt.Sprint(zoom),
			Resolution:       156543.03392804097 / factor,
			ScaleDenominator: 559082264.0287178 / factor,
			MatrixWidth:      int64(factor), MatrixHeight: int64(factor),
		})
		crs84.Levels = append(crs84.Levels, MatrixLevel{
			Zoom: zoom, Identifier: fmt.Sprint(zoom),
			Resolution:       0.703125 / factor,
			ScaleDenominator: 279541132.0143589 / factor,
			MatrixWidth:      int64(2 * factor), MatrixHeight: int64(factor),
		})
	}
	return []Scheme{mercator, crs84}
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tile_schemes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    crs TEXT NOT NULL,
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
    is_default INTEGER NOT NULL DEFAULT 0 CHECK(is_default IN (0, 1))
);

CREATE TABLE IF NOT EXISTS tile_matrix_levels (
    scheme_id TEXT NOT NULL REFERENCES tile_schemes(id) ON DELETE CASCADE,
    zoom INTEGER NOT NULL,
    identifier TEXT NOT NULL,
    resolution REAL NOT NULL CHECK(resolution > 0),
    scale_denominator REAL NOT NULL CHECK(scale_denominator > 0),
    matrix_width INTEGER NOT NULL CHECK(matrix_width > 0),
    matrix_height INTEGER NOT NULL CHECK(matrix_height > 0),
    PRIMARY KEY(scheme_id, zoom),
    UNIQUE(scheme_id, identifier)
);

CREATE INDEX IF NOT EXISTS idx_tile_matrix_levels_scheme
ON tile_matrix_levels(scheme_id, zoom);
`
