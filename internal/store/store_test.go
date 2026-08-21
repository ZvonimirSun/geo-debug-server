package store

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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
	if len(schemes) != 3 {
		t.Fatalf("expected 3 default schemes, got %d", len(schemes))
	}
	mercator, err := database.Scheme(ctx, WebMercatorQuad)
	if err != nil {
		t.Fatal(err)
	}
	if mercator.IsDefault || mercator.YCoordinateFirst || len(mercator.Levels) != 23 {
		t.Fatalf("unexpected default scheme: default=%v yFirst=%v levels=%d",
			mercator.IsDefault, mercator.YCoordinateFirst, len(mercator.Levels))
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
	if !ok || crs84.YCoordinateFirst || len(crs84.Levels) != 24 || crsLevel0.Resolution != 1.40625 ||
		crsLevel0.MatrixWidth != 1 || crsLevel0.MatrixHeight != 1 {
		t.Fatalf("unexpected WorldCRS84Quad level 0: %+v", crsLevel0)
	}
	crsLevel1, ok := crs84.Level("1")
	if !ok || crsLevel1.Resolution != 0.703125 || crsLevel1.MatrixWidth != 2 || crsLevel1.MatrixHeight != 1 {
		t.Fatalf("unexpected WorldCRS84Quad level 1: %+v", crsLevel1)
	}
	rightHalf := crs84.TileBounds(crsLevel1, 1, 0)
	if rightHalf.MinX != 0 || rightHalf.MaxX != 180 || rightHalf.MinY != -90 || rightHalf.MaxY != 90 {
		t.Fatalf("unexpected CRS84 right-half bounds: %+v", rightHalf)
	}

	cgcs2000, err := database.Scheme(ctx, CGCS2000Quad)
	if err != nil {
		t.Fatal(err)
	}
	cgcsLevel0, ok := cgcs2000.Level("0")
	if !ok || cgcs2000.CRS != "EPSG:4490" || !cgcs2000.IsDefault || !cgcs2000.YCoordinateFirst || len(cgcs2000.Levels) != 24 ||
		cgcsLevel0.Resolution != 1.40625 || cgcsLevel0.MatrixWidth != 1 || cgcsLevel0.MatrixHeight != 1 {
		t.Fatalf("unexpected CGCS2000Quad level 0: scheme=%+v level=%+v", cgcs2000, cgcsLevel0)
	}
	cgcsLevel1, ok := cgcs2000.Level("1")
	if !ok {
		t.Fatal("missing CGCS2000Quad level 1")
	}
	cgcsRightHalf := cgcs2000.TileBounds(cgcsLevel1, 1, 0)
	if cgcsRightHalf.MinX != 0 || cgcsRightHalf.MaxX != 180 || cgcsRightHalf.MinY != -90 || cgcsRightHalf.MaxY != 90 {
		t.Fatalf("unexpected CGCS2000 right-half bounds: %+v", cgcsRightHalf)
	}
}

func TestExistingDatabaseIsNotReinitialized(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "preserve.db")
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(ctx, `UPDATE tile_schemes SET name = 'Custom Name' WHERE id = ?`, WebMercatorQuad); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(ctx, `DELETE FROM tile_schemes WHERE id = ?`, CGCS2000Quad); err != nil {
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
	if _, err := database.Scheme(ctx, CGCS2000Quad); !errors.Is(err, ErrSchemeNotFound) {
		t.Fatalf("existing database unexpectedly restored the deleted scheme: %v", err)
	}
}

func TestExistingDatabaseDoesNotRestoreMissingLevels(t *testing.T) {
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
	if len(scheme.Levels) != 22 {
		t.Fatalf("existing database unexpectedly restored a missing level, got %d levels", len(scheme.Levels))
	}
	if _, ok := scheme.Level("7"); ok {
		t.Fatal("existing database unexpectedly restored level 7")
	}
	level8, ok := scheme.Level("8")
	if !ok || level8.Resolution != 123.5 {
		t.Fatalf("existing matrix level was overwritten: %+v", level8)
	}
}

func TestExistingSQLiteFileIsNotInitialized(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "existing-empty.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var tableCount int
	if err := database.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'tile_schemes'`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 0 {
		t.Fatal("existing SQLite file was unexpectedly initialized")
	}
}

func TestSchemeManagement(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "management.db")
	database, err := OpenWithCacheTTL(ctx, path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	scheme := testScheme("LocalQuad")
	if err := database.CreateScheme(ctx, scheme); err != nil {
		t.Fatal(err)
	}
	created, err := database.Scheme(ctx, strings.ToLower(scheme.ID))
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != scheme.Name || len(created.Levels) != 2 || created.IsDefault {
		t.Fatalf("unexpected created scheme: %+v", created)
	}
	if err := database.CreateScheme(ctx, testScheme("localquad")); !errors.Is(err, ErrSchemeExists) {
		t.Fatalf("expected case-insensitive duplicate error, got %v", err)
	}

	if _, err := database.DefaultScheme(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.SetDefaultScheme(ctx, strings.ToLower(scheme.ID)); err != nil {
		t.Fatal(err)
	}
	defaultScheme, err := database.DefaultScheme(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if defaultScheme.ID != scheme.ID || !defaultScheme.IsDefault {
		t.Fatalf("unexpected updated default scheme: %+v", defaultScheme)
	}
	oldDefault, err := database.Scheme(ctx, CGCS2000Quad)
	if err != nil {
		t.Fatal(err)
	}
	if oldDefault.IsDefault {
		t.Fatal("setting a default scheme left the previous default cached")
	}
	if err := database.SetDefaultScheme(ctx, "missing"); !errors.Is(err, ErrSchemeNotFound) {
		t.Fatalf("expected missing default target error, got %v", err)
	}

	if err := database.DeleteScheme(ctx, strings.ToLower(scheme.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Scheme(ctx, scheme.ID); !errors.Is(err, ErrSchemeNotFound) {
		t.Fatalf("deleted scheme is still available: %v", err)
	}
	defaultScheme, err = database.DefaultScheme(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if defaultScheme.ID != CGCS2000Quad || !defaultScheme.IsDefault {
		t.Fatalf("unexpected replacement default scheme: %+v", defaultScheme)
	}
	var levelCount int
	if err := database.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tile_matrix_levels WHERE scheme_id = ?`, scheme.ID).Scan(&levelCount); err != nil {
		t.Fatal(err)
	}
	if levelCount != 0 {
		t.Fatalf("deleted scheme retained %d levels", levelCount)
	}
	if err := database.DeleteScheme(ctx, "missing"); !errors.Is(err, ErrSchemeNotFound) {
		t.Fatalf("expected missing delete target error, got %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Scheme(ctx, scheme.ID); !errors.Is(err, ErrSchemeNotFound) {
		t.Fatalf("restart restored a deleted custom scheme: %v", err)
	}
}

func TestCreateSchemeValidationRollsBack(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "invalid-scheme.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	invalid := testScheme("InvalidQuad")
	invalid.Levels = invalid.Levels[:1]
	if err := database.CreateScheme(ctx, invalid); !errors.Is(err, ErrInvalidScheme) {
		t.Fatalf("expected invalid scheme error, got %v", err)
	}
	if _, err := database.Scheme(ctx, invalid.ID); !errors.Is(err, ErrSchemeNotFound) {
		t.Fatalf("invalid scheme was partially inserted: %v", err)
	}
}

func testScheme(id string) Scheme {
	return Scheme{
		ID: id, Name: "Local Quad", CRS: "EPSG:4326", MetersPerUnit: 111319.49079327358,
		TileWidth: 256, TileHeight: 256, MinZoom: 0, MaxZoom: 1,
		OriginX: -180, OriginY: 90, MinX: -180, MinY: -90, MaxX: 180, MaxY: 90,
		YCoordinateFirst: true,
		Levels: []MatrixLevel{
			{Zoom: 0, Identifier: "0", Resolution: 1.40625, MatrixWidth: 1, MatrixHeight: 1},
			{Zoom: 1, Identifier: "1", Resolution: 0.703125, MatrixWidth: 2, MatrixHeight: 1},
		},
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

func TestSchemeCacheUsesSlidingExpiration(t *testing.T) {
	ctx := context.Background()
	database, err := OpenWithCacheTTL(ctx, filepath.Join(t.TempDir(), "cache.db"), 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	first, err := database.Scheme(ctx, WebMercatorQuad)
	if err != nil {
		t.Fatal(err)
	}
	first.Levels[0].Resolution = -1
	if _, err := database.db.ExecContext(ctx, `UPDATE tile_schemes SET name = 'Database Name' WHERE id = ?`, WebMercatorQuad); err != nil {
		t.Fatal(err)
	}

	time.Sleep(60 * time.Millisecond)
	second, err := database.Scheme(ctx, WebMercatorQuad)
	if err != nil {
		t.Fatal(err)
	}
	if second.Name == "Database Name" {
		t.Fatal("scheme was reloaded before its sliding TTL expired")
	}
	if second.Levels[0].Resolution <= 0 {
		t.Fatal("cached scheme shared its Levels slice with the caller")
	}

	time.Sleep(60 * time.Millisecond)
	third, err := database.Scheme(ctx, strings.ToLower(WebMercatorQuad))
	if err != nil {
		t.Fatal(err)
	}
	if third.Name == "Database Name" {
		t.Fatal("case-insensitive cache hit did not extend the TTL")
	}

	waitFor(t, time.Second, func() bool {
		database.cacheMu.Lock()
		defer database.cacheMu.Unlock()
		return len(database.cache) == 0
	})
	reloaded, err := database.Scheme(ctx, WebMercatorQuad)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Name != "Database Name" {
		t.Fatalf("expired scheme was not reloaded from SQLite: %q", reloaded.Name)
	}
	database.cacheMu.Lock()
	loads := database.schemeLoads
	database.cacheMu.Unlock()
	if loads != 2 {
		t.Fatalf("expected two SQLite scheme loads, got %d", loads)
	}
}

func TestConcurrentSchemeCacheMissLoadsOnce(t *testing.T) {
	database, err := OpenWithCacheTTL(context.Background(), filepath.Join(t.TempDir(), "concurrent.db"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	const workers = 64
	start := make(chan struct{})
	errorsChannel := make(chan error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			<-start
			scheme, err := database.Scheme(context.Background(), WebMercatorQuad)
			if err == nil && len(scheme.Levels) != 23 {
				err = errors.New("scheme has incomplete matrix levels")
			}
			errorsChannel <- err
		}()
	}
	close(start)
	group.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	database.cacheMu.Lock()
	loads := database.schemeLoads
	database.cacheMu.Unlock()
	if loads != 1 {
		t.Fatalf("concurrent cache miss loaded SQLite %d times", loads)
	}
}

func TestConcurrentSchemeCacheHitsReuseTimer(t *testing.T) {
	database, err := OpenWithCacheTTL(context.Background(), filepath.Join(t.TempDir(), "cache-hits.db"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Scheme(context.Background(), WebMercatorQuad); err != nil {
		t.Fatal(err)
	}

	key := schemeCacheKey(WebMercatorQuad)
	database.cacheMu.RLock()
	initialTimer := database.cache[key].timer
	database.cacheMu.RUnlock()

	const workers = 128
	const hitsPerWorker = 100
	start := make(chan struct{})
	errorsChannel := make(chan error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			<-start
			for range hitsPerWorker {
				if _, err := database.Scheme(context.Background(), WebMercatorQuad); err != nil {
					errorsChannel <- err
					return
				}
			}
			errorsChannel <- nil
		}()
	}
	close(start)
	group.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}

	database.cacheMu.RLock()
	entry := database.cache[key]
	loads := database.schemeLoads
	database.cacheMu.RUnlock()
	if entry == nil || entry.timer != initialTimer {
		t.Fatal("cache hits replaced the entry expiration timer")
	}
	if loads != 1 {
		t.Fatalf("cache hits loaded SQLite %d times", loads)
	}
}

func BenchmarkSchemeCacheHitParallel(b *testing.B) {
	database, err := OpenWithCacheTTL(context.Background(), filepath.Join(b.TempDir(), "cache-benchmark.db"), time.Minute)
	if err != nil {
		b.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Scheme(context.Background(), WebMercatorQuad); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			if _, err := database.Scheme(context.Background(), WebMercatorQuad); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

func TestDefaultSchemeCacheExpiresAndReloadsSelection(t *testing.T) {
	ctx := context.Background()
	database, err := OpenWithCacheTTL(ctx, filepath.Join(t.TempDir(), "default-cache.db"), 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	initial, err := database.DefaultScheme(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if initial.ID != CGCS2000Quad {
		t.Fatalf("unexpected initial default: %s", initial.ID)
	}
	if _, err := database.db.ExecContext(ctx, `
		UPDATE tile_schemes SET is_default = CASE WHEN id = ? THEN 1 ELSE 0 END`, WorldCRS84Quad); err != nil {
		t.Fatal(err)
	}
	cached, err := database.DefaultScheme(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cached.ID != CGCS2000Quad {
		t.Fatalf("default selection changed before cache expiration: %s", cached.ID)
	}

	time.Sleep(80 * time.Millisecond)
	reloaded, err := database.DefaultScheme(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ID != WorldCRS84Quad {
		t.Fatalf("default selection was not reloaded after expiration: %s", reloaded.ID)
	}
}

func TestStoreCloseClearsSchemeCache(t *testing.T) {
	database, err := OpenWithCacheTTL(context.Background(), filepath.Join(t.TempDir(), "close.db"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Scheme(context.Background(), WebMercatorQuad); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("repeated close must be safe: %v", err)
	}
	database.cacheMu.Lock()
	cacheSize := len(database.cache)
	database.cacheMu.Unlock()
	if cacheSize != 0 {
		t.Fatalf("cache was not cleared on close: %d entries", cacheSize)
	}
	if _, err := database.Scheme(context.Background(), WebMercatorQuad); !errors.Is(err, ErrStoreClosed) {
		t.Fatalf("expected ErrStoreClosed after close, got %v", err)
	}
	if _, err := database.Schemes(context.Background()); !errors.Is(err, ErrStoreClosed) {
		t.Fatalf("expected ErrStoreClosed from Schemes after close, got %v", err)
	}
}

func TestSchemeCacheKeyMatchesSQLiteLookupSemantics(t *testing.T) {
	database, err := OpenWithCacheTTL(context.Background(), filepath.Join(t.TempDir(), "cache-key.db"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Scheme(context.Background(), WebMercatorQuad); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Scheme(context.Background(), " "+WebMercatorQuad+" "); !errors.Is(err, ErrSchemeNotFound) {
		t.Fatalf("whitespace-padded ID must not hit the cache: %v", err)
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
