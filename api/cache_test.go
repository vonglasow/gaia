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

func TestResolveProvider_Boundaries(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		expected string
	}{
		{
			name:     "openai with exact match",
			host:     "api.openai.com",
			port:     443,
			expected: "openai",
		},
		{
			name:     "openai with whitespace",
			host:     "  api.openai.com  ",
			port:     443,
			expected: "openai",
		},
		{
			name:     "openai wrong port",
			host:     "api.openai.com",
			port:     8080,
			expected: "ollama",
		},
		{
			name:     "localhost",
			host:     "localhost",
			port:     11434,
			expected: "ollama",
		},
		{
			name:     "empty host",
			host:     "",
			port:     443,
			expected: "ollama",
		},
		{
			name:     "zero port",
			host:     "api.openai.com",
			port:     0,
			expected: "ollama",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Set("host", tt.host)
			viper.Set("port", tt.port)
			got := resolveProvider()
			if got != tt.expected {
				t.Errorf("resolveProvider() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetCacheDir_FallbackToHomeDir(t *testing.T) {
	oldCacheDir := viper.GetString("cache.dir")
	viper.Set("cache.dir", "")
	t.Cleanup(func() {
		viper.Set("cache.dir", oldCacheDir)
	})

	cacheDir, err := getCacheDir()
	require.NoError(t, err)
	assert.Contains(t, cacheDir, ".config/gaia/cache")
}

func TestGetCacheDir_CustomPath(t *testing.T) {
	customPath := "/custom/cache/path"
	oldCacheDir := viper.GetString("cache.dir")
	viper.Set("cache.dir", customPath)
	t.Cleanup(func() {
		viper.Set("cache.dir", oldCacheDir)
	})

	cacheDir, err := getCacheDir()
	require.NoError(t, err)
	assert.Equal(t, customPath, cacheDir)
}

func TestGetCacheDir_WithWhitespace(t *testing.T) {
	customPath := "  /custom/cache/path  "
	oldCacheDir := viper.GetString("cache.dir")
	viper.Set("cache.dir", customPath)
	t.Cleanup(func() {
		viper.Set("cache.dir", oldCacheDir)
	})

	cacheDir, err := getCacheDir()
	require.NoError(t, err)
	assert.Equal(t, "/custom/cache/path", cacheDir)
}

func TestReadCache_Boundaries(t *testing.T) {
	tempDir := t.TempDir()
	viper.Set("cache.dir", tempDir)

	// Test reading non-existent key
	_, ok, err := readCache("nonexistent")
	require.NoError(t, err)
	assert.False(t, ok)

	// Test reading with empty key
	_, ok, err = readCache("")
	require.NoError(t, err)
	assert.False(t, ok)

	// Write and read empty response
	require.NoError(t, writeCache("empty-key", ""))
	response, ok, err := readCache("empty-key")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "", response)

	// Write and read very long response
	longResponse := string(make([]byte, 10000))
	require.NoError(t, writeCache("long-key", longResponse))
	response, ok, err = readCache("long-key")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, longResponse, response)
}

func TestWriteCache_Boundaries(t *testing.T) {
	tempDir := t.TempDir()
	viper.Set("cache.dir", tempDir)

	// Test writing empty key
	err := writeCache("", "response")
	require.NoError(t, err)

	// Test writing empty response
	err = writeCache("key", "")
	require.NoError(t, err)

	// Test overwriting existing key
	require.NoError(t, writeCache("key", "response1"))
	require.NoError(t, writeCache("key", "response2"))
	response, ok, err := readCache("key")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "response2", response)
}

func TestCacheStats_EmptyCache(t *testing.T) {
	tempDir := t.TempDir()
	viper.Set("cache.dir", tempDir)

	stats, err := CacheStats()
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Count)
	assert.Equal(t, int64(0), stats.SizeBytes)
}

func TestCacheStats_MissingDirectory(t *testing.T) {
	tempDir := t.TempDir()
	viper.Set("cache.dir", filepath.Join(tempDir, "nonexistent"))

	stats, err := CacheStats()
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Count)
	assert.Equal(t, int64(0), stats.SizeBytes)
}

func TestClearCache_EmptyCache(t *testing.T) {
	tempDir := t.TempDir()
	viper.Set("cache.dir", tempDir)

	removed, err := ClearCache()
	require.NoError(t, err)
	assert.Equal(t, 0, removed)
}

func TestClearCache_MissingDirectory(t *testing.T) {
	tempDir := t.TempDir()
	viper.Set("cache.dir", filepath.Join(tempDir, "nonexistent"))

	removed, err := ClearCache()
	require.NoError(t, err)
	assert.Equal(t, 0, removed)
}

func TestListCacheEntries_IgnoresNonJsonFiles(t *testing.T) {
	tempDir := t.TempDir()
	viper.Set("cache.dir", tempDir)

	// Create a non-JSON file
	nonJSONPath := filepath.Join(tempDir, "not-a-cache.txt")
	require.NoError(t, os.WriteFile(nonJSONPath, []byte("test"), 0600))

	// Create a valid cache entry
	require.NoError(t, writeCache("valid-key", "valid-response"))

	entries, err := ListCacheEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "valid-key", entries[0].Key)
}

func TestListCacheEntries_IgnoresSubdirectories(t *testing.T) {
	tempDir := t.TempDir()
	viper.Set("cache.dir", tempDir)

	// Create a subdirectory
	subDir := filepath.Join(tempDir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0755))

	// Create a valid cache entry
	require.NoError(t, writeCache("valid-key", "valid-response"))

	entries, err := ListCacheEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestReadCacheEntries_EmptyCache(t *testing.T) {
	tempDir := t.TempDir()
	viper.Set("cache.dir", tempDir)

	entries, err := ReadCacheEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 0)
}
