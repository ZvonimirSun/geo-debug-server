package store

import (
	"context"
	"errors"
	"math"
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
	if initial.ID != WebMercatorQuad {
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
	if cached.ID != WebMercatorQuad {
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
