package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
)

func (s *Store) CreateScheme(ctx context.Context, scheme Scheme) error {
	if err := validateScheme(scheme); err != nil {
		return err
	}
	if err := s.ensureOpen(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create tile scheme: %w", err)
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tile_schemes`).Scan(&count); err != nil {
		return fmt.Errorf("count tile schemes: %w", err)
	}
	if count == 0 {
		scheme.IsDefault = true
	}
	if err := createSchemeTx(ctx, tx, scheme); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit create tile scheme: %w", err)
	}
	s.invalidateSchemeCache()
	return nil
}

func createSchemeTx(ctx context.Context, tx *sql.Tx, scheme Scheme) error {
	var existingID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM tile_schemes WHERE lower(id) = lower(?)`, scheme.ID).Scan(&existingID)
	if err == nil {
		return fmt.Errorf("%w: %s", ErrSchemeExists, existingID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check tile scheme existence: %w", err)
	}
	if scheme.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE tile_schemes SET is_default = 0 WHERE is_default <> 0`); err != nil {
			return fmt.Errorf("clear default tile scheme: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tile_schemes(
			id, name, crs, meters_per_unit, tile_width, tile_height, min_zoom, max_zoom,
			origin_x, origin_y, min_x, min_y, max_x, max_y, y_coordinate_first, is_default
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		scheme.ID, scheme.Name, scheme.CRS, scheme.MetersPerUnit, scheme.TileWidth, scheme.TileHeight,
		scheme.MinZoom, scheme.MaxZoom, scheme.OriginX, scheme.OriginY,
		scheme.MinX, scheme.MinY, scheme.MaxX, scheme.MaxY, scheme.YCoordinateFirst, scheme.IsDefault); err != nil {
		return fmt.Errorf("insert tile scheme: %w", err)
	}
	for _, level := range scheme.Levels {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tile_matrix_levels(
				scheme_id, zoom, identifier, resolution, matrix_width, matrix_height
			) VALUES(?, ?, ?, ?, ?, ?)`,
			scheme.ID, level.Zoom, level.Identifier, level.Resolution,
			level.MatrixWidth, level.MatrixHeight); err != nil {
			return fmt.Errorf("insert tile matrix %s/%d: %w", scheme.ID, level.Zoom, err)
		}
	}
	return nil
}

func (s *Store) DeleteScheme(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidScheme)
	}
	if err := s.ensureOpen(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete tile scheme: %w", err)
	}
	defer tx.Rollback()
	var actualID string
	var isDefault bool
	err = tx.QueryRowContext(ctx, `SELECT id, is_default FROM tile_schemes WHERE lower(id) = lower(?)`, id).Scan(&actualID, &isDefault)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSchemeNotFound
	}
	if err != nil {
		return fmt.Errorf("query tile scheme for deletion: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tile_schemes WHERE id = ?`, actualID); err != nil {
		return fmt.Errorf("delete tile scheme: %w", err)
	}
	if isDefault {
		if _, err := tx.ExecContext(ctx, `
			UPDATE tile_schemes
			SET is_default = CASE WHEN id = (SELECT id FROM tile_schemes ORDER BY id LIMIT 1) THEN 1 ELSE 0 END`); err != nil {
			return fmt.Errorf("select replacement default tile scheme: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete tile scheme: %w", err)
	}
	s.invalidateSchemeCache()
	return nil
}

func (s *Store) SetDefaultScheme(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidScheme)
	}
	if err := s.ensureOpen(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set default tile scheme: %w", err)
	}
	defer tx.Rollback()
	var actualID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM tile_schemes WHERE lower(id) = lower(?)`, id).Scan(&actualID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSchemeNotFound
	}
	if err != nil {
		return fmt.Errorf("query default tile scheme target: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE tile_schemes SET is_default = CASE WHEN id = ? THEN 1 ELSE 0 END`, actualID); err != nil {
		return fmt.Errorf("set default tile scheme: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set default tile scheme: %w", err)
	}
	s.invalidateSchemeCache()
	return nil
}

func validateScheme(scheme Scheme) error {
	invalid := func(message string) error {
		return fmt.Errorf("%w: %s", ErrInvalidScheme, message)
	}
	if scheme.ID == "" || scheme.ID != strings.TrimSpace(scheme.ID) || strings.ContainsAny(scheme.ID, `/\\`) {
		return invalid("id must be non-empty, trimmed, and contain no path separators")
	}
	if strings.TrimSpace(scheme.Name) == "" {
		return invalid("name is required")
	}
	if strings.TrimSpace(scheme.CRS) == "" {
		return invalid("crs is required")
	}
	if !finitePositive(scheme.MetersPerUnit) {
		return invalid("metersPerUnit must be a finite positive number")
	}
	if scheme.TileWidth <= 0 || scheme.TileHeight <= 0 {
		return invalid("tileWidth and tileHeight must be positive")
	}
	if scheme.MinZoom < 0 || scheme.MaxZoom < scheme.MinZoom {
		return invalid("minZoom and maxZoom are invalid")
	}
	coordinates := []float64{scheme.OriginX, scheme.OriginY, scheme.MinX, scheme.MinY, scheme.MaxX, scheme.MaxY}
	for _, coordinate := range coordinates {
		if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
			return invalid("origin and extent coordinates must be finite")
		}
	}
	if scheme.MinX >= scheme.MaxX || scheme.MinY >= scheme.MaxY {
		return invalid("extent minimums must be less than maximums")
	}
	expectedLevels := scheme.MaxZoom - scheme.MinZoom + 1
	if len(scheme.Levels) != expectedLevels {
		return invalid(fmt.Sprintf("levels must contain every zoom from %d through %d", scheme.MinZoom, scheme.MaxZoom))
	}
	zooms := make(map[int]bool, len(scheme.Levels))
	identifiers := make(map[string]bool, len(scheme.Levels))
	for _, level := range scheme.Levels {
		if level.Zoom < scheme.MinZoom || level.Zoom > scheme.MaxZoom || zooms[level.Zoom] {
			return invalid("level zooms must be unique and within the scheme range")
		}
		if strings.TrimSpace(level.Identifier) == "" || identifiers[level.Identifier] {
			return invalid("level identifiers must be non-empty and unique")
		}
		if !finitePositive(level.Resolution) || level.MatrixWidth <= 0 || level.MatrixHeight <= 0 {
			return invalid("level resolution and matrix dimensions must be positive")
		}
		zooms[level.Zoom] = true
		identifiers[level.Identifier] = true
	}
	return nil
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (s *Store) ensureOpen() error {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	if s.closed {
		return ErrStoreClosed
	}
	return nil
}

func (s *Store) invalidateSchemeCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	for key, entry := range s.cache {
		if entry.timer != nil {
			entry.timer.Stop()
		}
		delete(s.cache, key)
	}
}
