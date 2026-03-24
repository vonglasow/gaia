package cache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Entry struct {
	Key       string    `json:"key"`
	Label     string    `json:"label"`
	PluginID  string    `json:"plugin_id"`
	Provider  string    `json:"provider"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	Response  string    `json:"response"`
	CreatedAt time.Time `json:"created_at"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type EntryInfo struct {
	Key       string
	Label     string
	PluginID  string
	Model     string
	CreatedAt time.Time
	SizeBytes int64
}

type Stats struct {
	Count     int
	SizeBytes int64
}

type KeyPayload struct {
	PluginID string    `json:"plugin_id"`
	Provider string    `json:"provider"`
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Label    string    `json:"label"`
}

func Enabled() bool {
	return viper.GetBool("cache.enabled")
}

func TTL() time.Duration {
	ttlSeconds := viper.GetInt("cache.ttl_seconds")
	if ttlSeconds <= 0 {
		return 0
	}
	return time.Duration(ttlSeconds) * time.Second
}

func BuildKey(payload KeyPayload) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(payload); err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:]), nil
}

func Get(key string) (Entry, bool, error) {
	path, err := entryPath(key)
	if err != nil {
		return Entry{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Entry{}, false, nil
		}
		return Entry{}, false, err
	}
	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return Entry{}, false, err
	}
	if expired(entry) {
		_ = os.Remove(path)
		return Entry{}, false, nil
	}
	return entry, true, nil
}

func Set(entry Entry) error {
	dir, err := getCacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, entry.Key+".json"), data, 0o600)
}

func List() ([]EntryInfo, error) {
	dir, err := getCacheDir()
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []EntryInfo{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("cache path %s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	results := []EntryInfo{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var cached Entry
		if err := json.Unmarshal(data, &cached); err != nil {
			return nil, err
		}
		if expired(cached) {
			_ = os.Remove(path)
			continue
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return nil, err
		}
		results = append(results, EntryInfo{
			Key:       cached.Key,
			Label:     cached.Label,
			PluginID:  cached.PluginID,
			Model:     cached.Model,
			CreatedAt: cached.CreatedAt,
			SizeBytes: fileInfo.Size(),
		})
	}
	return results, nil
}

func StatsInfo() (Stats, error) {
	dir, err := getCacheDir()
	if err != nil {
		return Stats{}, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Stats{}, nil
		}
		return Stats{}, err
	}
	if !info.IsDir() {
		return Stats{}, fmt.Errorf("cache path %s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return Stats{}, err
		}
		var cached Entry
		if err := json.Unmarshal(data, &cached); err != nil {
			return Stats{}, err
		}
		if expired(cached) {
			_ = os.Remove(path)
			continue
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return Stats{}, err
		}
		stats.Count++
		stats.SizeBytes += fileInfo.Size()
	}
	return stats, nil
}

func ClearAll() (int, error) {
	dir, err := getCacheDir()
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("cache path %s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func Delete(key string) error {
	path, err := entryPath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func entryPath(key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("cache key is required")
	}
	dir, err := getCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, key+".json"), nil
}

func getCacheDir() (string, error) {
	dir := strings.TrimSpace(viper.GetString("cache.dir"))
	if dir != "" {
		return dir, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory for cache: %w", err)
	}
	return filepath.Join(homeDir, ".config", "gaia", "cache"), nil
}

func expired(entry Entry) bool {
	ttl := TTL()
	if ttl == 0 {
		return false
	}
	return time.Since(entry.CreatedAt) > ttl
}
