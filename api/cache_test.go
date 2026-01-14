package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListCacheEntries_EmptyWhenMissingDir(t *testing.T) {
	tempDir := t.TempDir()
	oldCacheDir := viper.GetString("cache.dir")
	viper.Set("cache.dir", filepath.Join(tempDir, "missing"))
	t.Cleanup(func() {
		viper.Set("cache.dir", oldCacheDir)
	})

	entries, err := ListCacheEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 0)
}

func TestListCacheEntries_Metadata(t *testing.T) {
	cacheDir := t.TempDir()
	oldCacheDir := viper.GetString("cache.dir")
	viper.Set("cache.dir", cacheDir)
	t.Cleanup(func() {
		viper.Set("cache.dir", oldCacheDir)
	})

	require.NoError(t, writeCache("key-one", "response-one"))
	require.NoError(t, writeCache("key-two", "response-two"))

	entries, err := ListCacheEntries()
	require.NoError(t, err)
	require.Len(t, entries, 2)

	entriesByKey := make(map[string]CacheEntryInfo, len(entries))
	for _, entry := range entries {
		entriesByKey[entry.Key] = entry
	}

	for _, key := range []string{"key-one", "key-two"} {
		entry, ok := entriesByKey[key]
		require.True(t, ok)
		assert.False(t, entry.CreatedAt.IsZero())
		assert.Greater(t, entry.SizeBytes, int64(0))
	}
}

func TestReadCacheEntries_ReturnsResponses(t *testing.T) {
	cacheDir := t.TempDir()
	oldCacheDir := viper.GetString("cache.dir")
	viper.Set("cache.dir", cacheDir)
	t.Cleanup(func() {
		viper.Set("cache.dir", oldCacheDir)
	})

	require.NoError(t, writeCache("key-one", "response-one"))
	require.NoError(t, writeCache("key-two", "response-two"))

	entries, err := ReadCacheEntries()
	require.NoError(t, err)
	require.Len(t, entries, 2)

	responses := make(map[string]string, len(entries))
	for _, entry := range entries {
		responses[entry.Key] = entry.Response
	}

	assert.Equal(t, "response-one", responses["key-one"])
	assert.Equal(t, "response-two", responses["key-two"])
}

func TestCacheEnabled(t *testing.T) {
	viper.Set("cache.enabled", true)
	if !cacheEnabled() {
		t.Fatalf("expected cache to be enabled")
	}

	viper.Set("cache.enabled", false)
	if cacheEnabled() {
		t.Fatalf("expected cache to be disabled when cache.enabled is false")
	}
}

func TestResolveProvider(t *testing.T) {
	viper.Set("host", "api.openai.com")
	viper.Set("port", 443)
	if provider := resolveProvider(); provider != "openai" {
		t.Fatalf("expected provider openai, got %s", provider)
	}

	viper.Set("host", "localhost")
	viper.Set("port", 11434)
	if provider := resolveProvider(); provider != "ollama" {
		t.Fatalf("expected provider ollama, got %s", provider)
	}
}

func TestGetCacheDir(t *testing.T) {
	tempDir := t.TempDir()
	viper.Set("cache.dir", tempDir)

	cacheDir, err := getCacheDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cacheDir != tempDir {
		t.Fatalf("expected cache dir %s, got %s", tempDir, cacheDir)
	}
}

func TestBuildCacheKey(t *testing.T) {
	resetChatHistory()
	tempDir := t.TempDir()
	viper.Set("cache.dir", tempDir)
	viper.Set("cache.enabled", true)
	viper.Set("host", "localhost")
	viper.Set("port", 11434)
	viper.Set("model", "mistral")
	viper.Set("systemrole", "default")
	viper.Set("roles.default", "Hello %s on %s")

	if err := os.Setenv("SHELL", "/bin/zsh"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}

	key1, err := buildCacheKey("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	key2, err := buildCacheKey("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key1 != key2 {
		t.Fatalf("expected deterministic key, got %s and %s", key1, key2)
	}

	viper.Set("model", "different-model")
	key3, err := buildCacheKey("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key1 == key3 {
		t.Fatalf("expected cache key to change with model")
	}
}

func TestReadWriteCache(t *testing.T) {
	tempDir := t.TempDir()
	viper.Set("cache.dir", tempDir)

	key := "test-key"
	response := "cached response"

	if err := writeCache(key, response); err != nil {
		t.Fatalf("writeCache failed: %v", err)
	}

	got, ok, err := readCache(key)
	if err != nil {
		t.Fatalf("readCache failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected cache hit")
	}
	if got != response {
		t.Fatalf("expected response %q, got %q", response, got)
	}

	_, ok, err = readCache("missing-key")
	if err != nil {
		t.Fatalf("readCache missing failed: %v", err)
	}
	if ok {
		t.Fatalf("expected cache miss")
	}
}

func TestCacheStatsAndClear(t *testing.T) {
	tempDir := t.TempDir()
	viper.Set("cache.dir", tempDir)

	if err := writeCache("key-one", "response-one"); err != nil {
		t.Fatalf("writeCache failed: %v", err)
	}
	if err := writeCache("key-two", "response-two"); err != nil {
		t.Fatalf("writeCache failed: %v", err)
	}

	stats, err := CacheStats()
	if err != nil {
		t.Fatalf("CacheStats failed: %v", err)
	}
	if stats.Count != 2 {
		t.Fatalf("expected 2 cache entries, got %d", stats.Count)
	}
	if stats.SizeBytes == 0 {
		t.Fatalf("expected non-zero cache size")
	}

	removed, err := ClearCache()
	if err != nil {
		t.Fatalf("ClearCache failed: %v", err)
	}
	if removed != 2 {
		t.Fatalf("expected 2 removed entries, got %d", removed)
	}

	stats, err = CacheStats()
	if err != nil {
		t.Fatalf("CacheStats failed: %v", err)
	}
	if stats.Count != 0 {
		t.Fatalf("expected 0 cache entries after clear, got %d", stats.Count)
	}
}
