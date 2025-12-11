package coupons

import (
	"fmt"
	"strings"
	"sync"

	"github.com/linxGnu/grocksdb"
)

// Manager backed by RocksDB column families file1,file2,file3
type Manager struct {
	db   *grocksdb.DB
	cf   map[string]*grocksdb.ColumnFamilyHandle // keys: "file1","file2","file3"
	mu   sync.RWMutex
	// no in-memory counts are stored by default; Snapshot builds them on demand
}

// NewManagerFromRocks opens an existing RocksDB at dbPath with column families:
// default, file1, file2, file3. It returns a Manager that can query the DB.
func NewManagerFromRocks(dbPath string) (*Manager, error) {
	opts := grocksdb.NewDefaultOptions()
	// If db doesn't exist, OpenDbColumnFamilies will return an error.
	// We expect the DB folder to already be produced by your loader.
	opts.SetCreateIfMissing(false)

	cfNames := []string{"default", "file1", "file2", "file3"}
	cfOpts := []*grocksdb.Options{opts, opts, opts, opts}

	db, handles, err := grocksdb.OpenDbColumnFamilies(opts, dbPath, cfNames, cfOpts)
	if err != nil {
		return nil, fmt.Errorf("open db with cfs: %w", err)
	}
	if len(handles) < 4 {
		// Defensive: ensure we have expected CF handles
		// Close any handles we have before returning error.
		for _, h := range handles {
			if h != nil {
				h.Destroy()
			}
		}
		db.Close()
		return nil, fmt.Errorf("unexpected column family handles count: %d", len(handles))
	}

	m := &Manager{
		db: db,
		cf: make(map[string]*grocksdb.ColumnFamilyHandle, 3),
	}
	// handles matched to cfNames order: index 0 = default
	m.cf["file1"] = handles[1]
	m.cf["file2"] = handles[2]
	m.cf["file3"] = handles[3]

	return m, nil
}

// Close closes the DB and destroys CF handles.
func (m *Manager) Close() {
	if m.db == nil {
		return
	}
	// destroy CF handles
	for _, h := range m.cf {
		if h != nil {
			h.Destroy()
		}
	}
	m.db.Close()
	m.db = nil
}

// IsValidPromo returns true if the code exists in at least 2 column families.
func (m *Manager) IsValidPromo(code string) bool {
	code = strings.TrimSpace(strings.ToUpper(code))
	if len(code) < 8 || len(code) > 10 {
		return false
	}
	ro := grocksdb.NewDefaultReadOptions()
	defer ro.Destroy()

	cnt := 0
	for _, name := range []string{"file1", "file2", "file3"} {
		c := m.cf[name]
		if c == nil {
			continue
		}
		val, err := m.db.GetCF(ro, c, []byte(code))
		if err == nil && val != nil && val.Size() > 0 {
			cnt++
			val.Free()
		} else if val != nil {
			// free even on error
			val.Free()
		}
		if cnt >= 2 {
			return true
		}
	}
	return false
}

// Snapshot returns a map[string]int with counts per code (same format as your old manager).
// WARNING: this loads every unique code into memory and may OOM for very large DBs.
func (m *Manager) Snapshot() map[string]int {
	ro := grocksdb.NewDefaultReadOptions()
	defer ro.Destroy()

	result := make(map[string]int, 1024)

	for _, cfName := range []string{"file1", "file2", "file3"} {
		cf := m.cf[cfName]
		if cf == nil {
			continue
		}
		it := m.db.NewIteratorCF(ro, cf)
		for it.SeekToFirst(); it.Valid(); it.Next() {
			k := it.Key()
			// copy key
			key := string(k.Data())
			k.Free()
			result[key]++
		}
		it.Close()
	}
	return result
}
