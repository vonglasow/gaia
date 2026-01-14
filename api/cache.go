package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type CacheStatsInfo struct {
	Count     int
	SizeBytes int64
}

type cacheEntry struct {
	Key       string    `json:"key"`
	Response  string    `json:"response"`
	CreatedAt time.Time `json:"created_at"`
}

type cacheKeyPayload struct {
	Provider     string    `json:"provider"`
	Host         string    `json:"host"`
	Port         int       `json:"port"`
	Model        string    `json:"model"`
	SystemRole   string    `json:"system_role"`
	RoleTemplate string    `json:"role_template"`
	Messages     []Message `json:"messages"`
}

func cacheEnabled() bool {
	if viper.GetBool("cache.bypass") {
		return false
	}
	return viper.GetBool("cache.enabled")
}

func resolveProvider() string {
	host := strings.TrimSpace(viper.GetString("host"))
	port := viper.GetInt("port")
	if strings.Contains(host, "api.openai.com") && port == 443 {
		return "openai"
	}
	return "ollama"
}

func getCacheDir() (string, error) {
	cacheDir := strings.TrimSpace(viper.GetString("cache.dir"))
	if cacheDir != "" {
		return cacheDir, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory for cache: %w", err)
	}
	return filepath.Join(homeDir, ".config", "gaia", "cache"), nil
}

func buildCacheKey(msg string) (string, error) {
	request, err := buildRequestPayload(msg)
	if err != nil {
		return "", err
	}

	systemRole := viper.GetString("systemrole")
	if systemRole == "" {
		systemRole = viper.GetString("role")
	}
	if systemRole == "" {
		systemRole = "default"
	}

	roleTemplate := viper.GetString(fmt.Sprintf("roles.%s", systemRole))

	payload := cacheKeyPayload{
		Provider:     resolveProvider(),
		Host:         viper.GetString("host"),
		Port:         viper.GetInt("port"),
		Model:        viper.GetString("model"),
		SystemRole:   systemRole,
		RoleTemplate: roleTemplate,
		Messages:     request.Messages,
	}

	keyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to encode cache key payload: %w", err)
	}
	sum := sha256.Sum256(keyBytes)
	return hex.EncodeToString(sum[:]), nil
}

func readCache(key string) (string, bool, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return "", false, err
	}
	cachePath := filepath.Join(cacheDir, key+".json")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return "", false, err
	}
	return entry.Response, true, nil
}

func writeCache(key, response string) error {
	cacheDir, err := getCacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	entry := cacheEntry{
		Key:       key,
		Response:  response,
		CreatedAt: time.Now().UTC(),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to encode cache entry: %w", err)
	}
	cachePath := filepath.Join(cacheDir, key+".json")
	return os.WriteFile(cachePath, data, 0o600)
}

func CacheStats() (CacheStatsInfo, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return CacheStatsInfo{}, err
	}
	info, err := os.Stat(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return CacheStatsInfo{}, nil
		}
		return CacheStatsInfo{}, err
	}
	if !info.IsDir() {
		return CacheStatsInfo{}, fmt.Errorf("cache path %s is not a directory", cacheDir)
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return CacheStatsInfo{}, err
	}

	var stats CacheStatsInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return CacheStatsInfo{}, err
		}
		stats.Count++
		stats.SizeBytes += fileInfo.Size()
	}
	return stats, nil
}

func ClearCache() (int, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("cache path %s is not a directory", cacheDir)
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if err := os.Remove(filepath.Join(cacheDir, entry.Name())); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
