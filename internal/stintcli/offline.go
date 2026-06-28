package stintcli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"
)

const offlineBucket = "heartbeats"
const offlineDedupeWindowSeconds = 1

func DefaultQueuePath() string {
	return filepath.Join(wakaResourcesDir(), "offline_heartbeats.bdb")
}

func DefaultLegacyQueuePath() string {
	if home := strings.TrimSpace(os.Getenv("WAKATIME_HOME")); home != "" {
		return filepath.Join(expandHome(home), ".wakatime.bdb")
	}
	return expandHome("~/.wakatime.bdb")
}

func defaultLegacyQueuePathIfEmpty(path string) string {
	if strings.TrimSpace(path) == "" {
		return DefaultLegacyQueuePath()
	}
	return path
}

func AppendQueue(path string, heartbeats []Heartbeat) error {
	path = expandHome(path)
	if legacyJSONL(path) {
		return appendJSONLQueue(path, heartbeats)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	db, err := openQueueDB(path, false)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(offlineBucket))
		if err != nil {
			return err
		}
		for _, hb := range heartbeats {
			data, err := json.Marshal(hb)
			if err != nil {
				return err
			}
			key := []byte(hb.ID())
			if existing := b.Get(key); existing != nil {
				key = []byte(hb.ID() + "-" + time.Now().Format("20060102150405.000000000"))
			}
			if err := b.Put(key, data); err != nil {
				return err
			}
		}
		return nil
	})
}

func appendJSONLQueue(path string, heartbeats []Heartbeat) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, hb := range heartbeats {
		if err := enc.Encode(hb); err != nil {
			return err
		}
	}
	return nil
}

func ReadQueue(path string, limit int) ([]Heartbeat, error) {
	path = expandHome(path)
	if legacyJSONL(path) {
		return readJSONLQueue(path, limit)
	}
	db, err := openQueueDB(path, true)
	if errors.Is(err, os.ErrNotExist) {
		return readDefaultLegacyQueue(path, limit)
	}
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var out []Heartbeat
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(offlineBucket))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for key, value := c.First(); key != nil; key, value = c.Next() {
			if limit > 0 && len(out) >= limit {
				break
			}
			var hb Heartbeat
			if err := json.Unmarshal(value, &hb); err != nil {
				return err
			}
			out = append(out, hb)
		}
		return nil
	})
	return out, err
}

func readJSONLQueue(path string, limit int) ([]Heartbeat, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Heartbeat
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if limit > 0 && len(out) >= limit {
			break
		}
		var hb Heartbeat
		if err := json.Unmarshal(scanner.Bytes(), &hb); err != nil {
			return nil, err
		}
		out = append(out, hb)
	}
	return out, scanner.Err()
}

func CountQueue(path string) (int, error) {
	path = expandHome(path)
	if legacyJSONL(path) {
		return countJSONLQueue(path)
	}
	db, err := openQueueDB(path, true)
	if errors.Is(err, os.ErrNotExist) {
		return countDefaultLegacyQueue(path)
	}
	if err != nil {
		return 0, err
	}
	defer db.Close()
	count := 0
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(offlineBucket))
		if b == nil {
			return nil
		}
		count = b.Stats().KeyN
		return nil
	})
	return count, err
}

func countJSONLQueue(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()
	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

func RemoveQueuePrefix(path string, n int) error {
	if n <= 0 {
		return nil
	}
	path = expandHome(path)
	if legacyJSONL(path) {
		return removeJSONLQueuePrefix(path, n)
	}
	db, err := openQueueDB(path, false)
	if errors.Is(err, os.ErrNotExist) {
		return removeDefaultLegacyQueuePrefix(path, n)
	}
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(offlineBucket))
		if err != nil {
			return err
		}
		c := b.Cursor()
		removed := 0
		for key, _ := c.First(); key != nil && removed < n; key, _ = c.Next() {
			if err := c.Delete(); err != nil {
				return err
			}
			removed++
		}
		return nil
	})
}

func DeleteQueueDuplicates(path string) (int, error) {
	path = expandHome(path)
	if legacyJSONL(path) {
		return deleteJSONLQueueDuplicates(path)
	}
	db, err := openQueueDB(path, false)
	if errors.Is(err, os.ErrNotExist) {
		return deleteDefaultLegacyQueueDuplicates(path)
	}
	if err != nil {
		return 0, err
	}
	defer db.Close()
	deleted := 0
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(offlineBucket))
		if err != nil {
			return err
		}
		kept := map[string][]float64{}
		c := b.Cursor()
		for key, value := c.First(); key != nil; key, value = c.Next() {
			var hb Heartbeat
			if err := json.Unmarshal(value, &hb); err != nil {
				return err
			}
			if duplicateQueuedHeartbeat(kept, hb) {
				if err := c.Delete(); err != nil {
					return err
				}
				deleted++
				continue
			}
			kept[hb.Entity] = append(kept[hb.Entity], hb.Time)
		}
		return nil
	})
	return deleted, err
}

func deleteJSONLQueueDuplicates(path string) (int, error) {
	heartbeats, err := readJSONLQueue(path, 0)
	if err != nil {
		return 0, err
	}
	if len(heartbeats) == 0 {
		return 0, nil
	}
	keptTimes := map[string][]float64{}
	keptHeartbeats := make([]Heartbeat, 0, len(heartbeats))
	deleted := 0
	for _, hb := range heartbeats {
		if duplicateQueuedHeartbeat(keptTimes, hb) {
			deleted++
			continue
		}
		keptTimes[hb.Entity] = append(keptTimes[hb.Entity], hb.Time)
		keptHeartbeats = append(keptHeartbeats, hb)
	}
	if deleted == 0 {
		return 0, nil
	}
	if err := rewriteJSONLQueue(path, keptHeartbeats); err != nil {
		return 0, err
	}
	return deleted, nil
}

func duplicateQueuedHeartbeat(kept map[string][]float64, hb Heartbeat) bool {
	for _, keptTime := range kept[hb.Entity] {
		if math.Abs(hb.Time-keptTime) <= offlineDedupeWindowSeconds {
			return true
		}
	}
	return false
}

func removeJSONLQueuePrefix(path string, n int) error {
	heartbeats, err := readJSONLQueue(path, 0)
	if err != nil {
		return err
	}
	if n >= len(heartbeats) {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return rewriteJSONLQueue(path, heartbeats[n:])
}

func rewriteJSONLQueue(path string, heartbeats []Heartbeat) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, hb := range heartbeats {
		if err := enc.Encode(hb); err != nil {
			return err
		}
	}
	return nil
}

func openQueueDB(path string, readOnly bool) (*bolt.DB, error) {
	if readOnly {
		if _, err := os.Stat(path); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: readOnly, Timeout: time.Second})
	if err == nil || !isCorruptQueueError(err) {
		return db, err
	}
	_, resetErr := resetCorruptQueue(path)
	if resetErr != nil {
		return nil, fmt.Errorf("failed to move corrupt offline queue %q: %w", path, resetErr)
	}
	if readOnly {
		return nil, os.ErrNotExist
	}
	return bolt.Open(path, 0o600, &bolt.Options{ReadOnly: readOnly, Timeout: time.Second})
}

func isCorruptQueueError(err error) bool {
	return errors.Is(err, bolterrors.ErrInvalid) ||
		errors.Is(err, bolterrors.ErrVersionMismatch) ||
		errors.Is(err, bolterrors.ErrChecksum)
}

func resetCorruptQueue(path string) (string, error) {
	backup := fmt.Sprintf("%s.corrupt.%s", path, time.Now().UTC().Format("20060102T150405.000000000Z"))
	err := os.Rename(path, backup)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return backup, nil
}

func legacyJSONL(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".jsonl")
}

func defaultLegacyQueuePath(path string) string {
	if filepath.Base(path) != "offline_heartbeats.bdb" {
		return ""
	}
	return filepath.Join(filepath.Dir(path), "offline_heartbeats.jsonl")
}

func readDefaultLegacyQueue(path string, limit int) ([]Heartbeat, error) {
	legacy := defaultLegacyQueuePath(path)
	if legacy == "" {
		return nil, nil
	}
	return readJSONLQueue(legacy, limit)
}

func countDefaultLegacyQueue(path string) (int, error) {
	legacy := defaultLegacyQueuePath(path)
	if legacy == "" {
		return 0, nil
	}
	return countJSONLQueue(legacy)
}

func removeDefaultLegacyQueuePrefix(path string, n int) error {
	legacy := defaultLegacyQueuePath(path)
	if legacy == "" {
		return nil
	}
	return removeJSONLQueuePrefix(legacy, n)
}

func deleteDefaultLegacyQueueDuplicates(path string) (int, error) {
	legacy := defaultLegacyQueuePath(path)
	if legacy == "" {
		return 0, nil
	}
	return deleteJSONLQueueDuplicates(legacy)
}
