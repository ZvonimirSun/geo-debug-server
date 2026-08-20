package store

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"testing"
)

func TestDefaultSchemesAndBounds(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "schemes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	schemes, err := database.Schemes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(schemes) != 2 {
		t.Fatalf("expected 2 default schemes, got %d", len(schemes))
	}
	mercator, err := database.Scheme(ctx, WebMercatorQuad)
	if err != nil {
		t.Fatal(err)
	}
	if !mercator.IsDefault || len(mercator.Levels) != 23 {
		t.Fatalf("unexpected default scheme: default=%v levels=%d", mercator.IsDefault, len(mercator.Levels))
	}
	level0, ok := mercator.Level("0")
	if !ok || level0.MatrixWidth != 1 || level0.MatrixHeight != 1 {
		t.Fatalf("unexpected WebMercatorQuad level 0: %+v", level0)
	}
	bounds := mercator.TileBounds(level0, 0, 0)
	if math.Abs(bounds.MinX-mercator.MinX) > 1e-6 || math.Abs(bounds.MaxY-mercator.MaxY) > 1e-6 {
		t.Fatalf("unexpected mercator bounds: %+v", bounds)
	}

	crs84, err := database.Scheme(ctx, WorldCRS84Quad)
	if err != nil {
		t.Fatal(err)
	}
	crsLevel0, ok := crs84.Level("0")
	if !ok || crsLevel0.MatrixWidth != 2 || crsLevel0.MatrixHeight != 1 {
		t.Fatalf("unexpected WorldCRS84Quad level 0: %+v", crsLevel0)
	}
	rightHalf := crs84.TileBounds(crsLevel0, 1, 0)
	if rightHalf.MinX != 0 || rightHalf.MaxX != 180 || rightHalf.MinY != -90 || rightHalf.MaxY != 90 {
		t.Fatalf("unexpected CRS84 right-half bounds: %+v", rightHalf)
	}
}

func TestInitializationDoesNotOverwriteExistingScheme(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "preserve.db")
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(ctx, `UPDATE tile_schemes SET name = 'Custom Name' WHERE id = ?`, WebMercatorQuad); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scheme, err := database.Scheme(ctx, WebMercatorQuad)
	if err != nil {
		t.Fatal(err)
	}
	if scheme.Name != "Custom Name" {
		t.Fatalf("initialization overwrote scheme name: %q", scheme.Name)
	}
}

func TestInitializationFillsMissingLevelsWithoutOverwritingExistingLevels(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "levels.db")
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(ctx, `DELETE FROM tile_matrix_levels WHERE scheme_id = ? AND zoom = 7`, WebMercatorQuad); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(ctx, `UPDATE tile_matrix_levels SET resolution = 123.5 WHERE scheme_id = ? AND zoom = 8`, WebMercatorQuad); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scheme, err := database.Scheme(ctx, WebMercatorQuad)
	if err != nil {
		t.Fatal(err)
	}
	if len(scheme.Levels) != 23 {
		t.Fatalf("expected missing level to be restored, got %d levels", len(scheme.Levels))
	}
	level8, ok := scheme.Level("8")
	if !ok || level8.Resolution != 123.5 {
		t.Fatalf("existing matrix level was overwritten: %+v", level8)
	}
}

func TestUnknownScheme(t *testing.T) {
	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "unknown.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	_, err = database.Scheme(context.Background(), "missing")
	if !errors.Is(err, ErrSchemeNotFound) {
		t.Fatalf("expected ErrSchemeNotFound, got %v", err)
	}
}
