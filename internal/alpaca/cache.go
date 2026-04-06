package alpaca

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
)

const (
	alpacaCacheDir               = ".cache/alpaca"
	symbolsCacheFile             = "symbols-v1.json"
	symbolsCacheMaxAge           = 12 * time.Hour
	historicalMinuteBarsCacheDir = "historical-minute-bars-v1"
	repoRootMarker               = "go.mod"
)

type cacheEnvelope[T any] struct {
	StoredAt time.Time `json:"stored_at"`
	Value    T         `json:"value"`
}

type historicalBarsCacheKey struct {
	Symbols    []string `json:"symbols"`
	TimeFrame  string   `json:"time_frame"`
	Adjustment string   `json:"adjustment"`
	Start      string   `json:"start"`
	End        string   `json:"end"`
	TotalLimit int      `json:"total_limit"`
	PageLimit  int      `json:"page_limit"`
	Feed       string   `json:"feed"`
	AsOf       string   `json:"as_of"`
	Currency   string   `json:"currency"`
	Sort       string   `json:"sort"`
	Version    string   `json:"version"`
}

var (
	repoRootOnce sync.Once
	repoRootPath string
	repoRootErr  error
)

func readCache[T any](relativePath string, maxAge time.Duration) (T, bool, error) {
	var zero T

	path, err := cacheFilePath(relativePath)
	if err != nil {
		return zero, false, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return zero, false, nil
		}
		return zero, false, err
	}

	var envelope cacheEnvelope[T]
	if err := json.Unmarshal(data, &envelope); err != nil {
		return zero, false, err
	}

	if maxAge > 0 && time.Since(envelope.StoredAt) > maxAge {
		return zero, false, nil
	}

	return envelope.Value, true, nil
}

func writeCache[T any](relativePath string, value T) (writeErr error) {
	path, err := cacheFilePath(relativePath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	payload, err := json.Marshal(cacheEnvelope[T]{
		StoredAt: time.Now().UTC(),
		Value:    value,
	})
	if err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}

	tempPath := tempFile.Name()
	defer func() {
		if tempPath == "" {
			return
		}
		if err := os.Remove(tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			writeErr = errors.Join(writeErr, err)
		}
	}()

	if _, err := tempFile.Write(payload); err != nil {
		if closeErr := tempFile.Close(); closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		return err
	}

	if err := os.Rename(tempPath, path); err != nil {
		return err
	}

	tempPath = ""
	return nil
}

func historicalBarsCachePath(symbols []string, request marketdata.GetBarsRequest) (string, error) {
	normalizedSymbols := append([]string(nil), symbols...)
	sort.Strings(normalizedSymbols)

	key, err := json.Marshal(historicalBarsCacheKey{
		Symbols:    normalizedSymbols,
		TimeFrame:  normalizedTimeFrame(request.TimeFrame).String(),
		Adjustment: string(normalizedAdjustment(request.Adjustment)),
		Start:      request.Start.UTC().Format(time.RFC3339Nano),
		End:        request.End.UTC().Format(time.RFC3339Nano),
		TotalLimit: request.TotalLimit,
		PageLimit:  request.PageLimit,
		Feed:       string(request.Feed),
		AsOf:       request.AsOf,
		Currency:   request.Currency,
		Sort:       string(request.Sort),
		Version:    historicalMinuteBarsCacheDir,
	})
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(key)
	return filepath.Join(historicalMinuteBarsCacheDir, hex.EncodeToString(sum[:])+".json"), nil
}

func cacheFilePath(relativePath string) (string, error) {
	repoRoot, err := repoRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(repoRoot, alpacaCacheDir, relativePath), nil
}

func repoRootDir() (string, error) {
	repoRootOnce.Do(func() {
		cwd, err := os.Getwd()
		if err != nil {
			repoRootErr = err
			return
		}

		dir := cwd
		for {
			markerPath := filepath.Join(dir, repoRootMarker)
			info, err := os.Stat(markerPath)
			if err == nil && !info.IsDir() {
				repoRootPath = dir
				return
			}
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				repoRootErr = err
				return
			}

			parent := filepath.Dir(dir)
			if parent == dir {
				repoRootErr = fmt.Errorf("%s not found from %s", repoRootMarker, cwd)
				return
			}
			dir = parent
		}
	})

	return repoRootPath, repoRootErr
}

func normalizedAdjustment(adjustment marketdata.Adjustment) marketdata.Adjustment {
	if adjustment == "" {
		return marketdata.AdjustmentRaw
	}
	return adjustment
}

func normalizedTimeFrame(timeFrame marketdata.TimeFrame) marketdata.TimeFrame {
	if timeFrame.N == 0 {
		return marketdata.OneDay
	}
	return timeFrame
}
