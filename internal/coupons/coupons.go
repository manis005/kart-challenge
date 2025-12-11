package coupons

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// -----------------------------------------------------------------------------
// Manager: maintains the coupon map + background reload loop
// -----------------------------------------------------------------------------

type Manager struct {
	counts         map[string]int
	mu             sync.RWMutex
	files          []string
	reloadInterval time.Duration
	quit           chan struct{}
	wg             sync.WaitGroup
}

// NewEmptyManager returns a manager with no coupons (used if files missing)
func NewEmptyManager() *Manager {
	return &Manager{
		counts: make(map[string]int),
		quit:   make(chan struct{}),
	}
}

// NewManagerFromFiles loads initial coupons AND starts background reloader.
func NewManagerFromFiles(files []string) (*Manager, error) {
	if len(files) < 3 {
		return nil, errors.New("need 3 coupon files")
	}

	m := &Manager{
		files:          append([]string(nil), files...),
		reloadInterval: time.Hour, // reload every hour
		quit:           make(chan struct{}),
	}

	// Initial load
	if err := m.reloadOnce(); err != nil {
		return nil, err
	}

	// Start background loop
	m.wg.Add(1)
	go m.reloadLoop()

	return m, nil
}

// Close stops background reloader (call on app shutdown)
func (m *Manager) Close() {
	close(m.quit)
	m.wg.Wait()
}

// IsValidPromo returns true if the code appears in >=2 files.
func (m *Manager) IsValidPromo(code string) bool {
	code = strings.TrimSpace(strings.ToUpper(code))
	if len(code) < 8 || len(code) > 10 {
		return false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.counts[code] >= 2
}

// Snapshot returns a copy (useful for debugging)
func (m *Manager) Snapshot() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cp := make(map[string]int, len(m.counts))
	for k, v := range m.counts {
		cp[k] = v
	}
	return cp
}

// -----------------------------------------------------------------------------
// Background reload loop
// -----------------------------------------------------------------------------

func (m *Manager) reloadLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.reloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.reloadOnce(); err != nil {
				log.Printf("[couponloader] reload error: %v", err)
			}
		case <-m.quit:
			return
		}
	}
}

// -----------------------------------------------------------------------------
// Reload logic (fast, concurrent)
// -----------------------------------------------------------------------------

const heuristicAverageBytesPerLine = 12 // cheap guess for preallocation

func (m *Manager) reloadOnce() error {
	type fileRes struct {
		idx     int
		path    string
		set     map[string]struct{}
		err     error
		elapsed time.Duration
	}

	n := len(m.files)
	results := make(chan fileRes, n)

	var wg sync.WaitGroup

	for i, path := range m.files {
		wg.Add(1)
		go func(idx int, p string) {
			defer wg.Done()
			start := time.Now()

			// Pre-size set based on gzip file size
			var set map[string]struct{}
			if fi, err := os.Stat(p); err == nil {
				est := int(fi.Size() / heuristicAverageBytesPerLine)
				if est > 0 && est < 20_000_000 {
					set = make(map[string]struct{}, est)
				} else {
					set = make(map[string]struct{}, 0)
				}
			} else {
				set = make(map[string]struct{}, 0)
			}

			err := fastReadGzipIntoSet(p, set)
			elapsed := time.Since(start)

			results <- fileRes{idx: idx, path: p, set: set, err: err, elapsed: elapsed}
		}(i, path)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	perFileSets := make([]map[string]struct{}, n)
	var anyErr error

	for r := range results {
		if r.err != nil {
			log.Printf("[couponloader] error reading %s: %v (elapsed %v)", r.path, r.err, r.elapsed)
			anyErr = r.err
		} else {
			log.Printf("[couponloader] read %d unique codes from %s in %v",
				len(r.set), r.path, r.elapsed)
		}
		perFileSets[r.idx] = r.set
	}

	if anyErr != nil {
		return anyErr
	}

	// Aggregate counts
	newCounts := make(map[string]int, 1024)
	for _, set := range perFileSets {
		for code := range set {
			newCounts[code]++
		}
	}

	// Swap atomically
	m.mu.Lock()
	m.counts = newCounts
	m.mu.Unlock()

	log.Printf("[couponloader] reload complete: %d total keys @ %s",
		len(newCounts), time.Now().Format(time.RFC3339))

	return nil
}

// -----------------------------------------------------------------------------
// Fast GZIP Reader (faster than Scanner for large files)
// -----------------------------------------------------------------------------

func fastReadGzipIntoSet(path string, set map[string]struct{}) error {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	const bufSize = 1 << 20 // 1MB buffer
	r := bufio.NewReaderSize(gz, bufSize)

	for {
		line, err := r.ReadString('\n')
		if line != "" {
			line = strings.TrimSpace(strings.ToUpper(line))
			if len(line) >= 8 && len(line) <= 10 {
				set[line] = struct{}{}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}
