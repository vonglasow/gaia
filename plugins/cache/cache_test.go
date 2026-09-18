package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// useTempCache points the store at a directory of the test's own and puts viper back.
func useTempCache(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cache")
	viper.Reset()
	viper.Set("cache.dir", dir)
	t.Cleanup(viper.Reset)
	return dir
}

func TestBuildKeyIsStableForTheSamePayload(t *testing.T) {
	payload := KeyPayload{
		PluginID: "ask",
		Provider: "ollama",
		Host:     "localhost",
		Port:     11434,
		Model:    "llama3.1",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}

	first, err := BuildKey(payload)
	require.NoError(t, err)
	second, err := BuildKey(payload)
	require.NoError(t, err)

	require.Equal(t, first, second, "the same question must land on the same entry")
	require.Len(t, first, 64, "the key is a hex sha256")
}

// A key that ignored part of the payload would serve an answer from the wrong model.
func TestBuildKeySeparatesPayloadsThatDiffer(t *testing.T) {
	base := KeyPayload{
		PluginID: "ask",
		Provider: "ollama",
		Host:     "localhost",
		Port:     11434,
		Model:    "llama3.1",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}
	baseKey, err := BuildKey(base)
	require.NoError(t, err)

	for name, mutate := range map[string]func(*KeyPayload){
		"a different model":    func(p *KeyPayload) { p.Model = "mistral" },
		"a different provider": func(p *KeyPayload) { p.Provider = "openai" },
		"a different host":     func(p *KeyPayload) { p.Host = "10.0.0.1" },
		"a different port":     func(p *KeyPayload) { p.Port = 8080 },
		"a different plugin":   func(p *KeyPayload) { p.PluginID = "chat" },
		"a different label":    func(p *KeyPayload) { p.Label = "other" },
		"a different message":  func(p *KeyPayload) { p.Messages[0].Content = "goodbye" },
		"an extra message": func(p *KeyPayload) {
			p.Messages = append(p.Messages, Message{Role: "user", Content: "again"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := base
			changed.Messages = append([]Message(nil), base.Messages...)
			mutate(&changed)

			other, err := BuildKey(changed)
			require.NoError(t, err)
			require.NotEqual(t, baseKey, other)
		})
	}
}

func TestSetThenGetReturnsWhatWasStored(t *testing.T) {
	useTempCache(t)

	require.NoError(t, Set(Entry{
		Key:      "abc",
		Label:    "a question",
		PluginID: "ask",
		Model:    "llama3.1",
		Response: "an answer",
	}))

	entry, found, err := Get("abc")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "an answer", entry.Response)
	require.False(t, entry.CreatedAt.IsZero(), "Set stamps an entry that arrived without a time")
}

func TestGetReportsAMissWithoutAnError(t *testing.T) {
	useTempCache(t)

	_, found, err := Get("never-written")
	require.NoError(t, err, "an empty cache is not a failure")
	require.False(t, found)
}

func TestGetRefusesAnEmptyKey(t *testing.T) {
	useTempCache(t)

	_, _, err := Get("   ")
	require.Error(t, err)
}

// Expiry is what makes a stale answer disappear rather than being served forever.
func TestGetDropsAnEntryPastItsTTL(t *testing.T) {
	dir := useTempCache(t)
	viper.Set("cache.ttl_seconds", 60)

	require.NoError(t, Set(Entry{
		Key:       "stale",
		Response:  "old news",
		CreatedAt: time.Now().Add(-2 * time.Hour),
	}))

	_, found, err := Get("stale")
	require.NoError(t, err)
	require.False(t, found)

	_, statErr := os.Stat(filepath.Join(dir, "stale.json"))
	require.True(t, os.IsNotExist(statErr), "the expired entry is removed, not merely hidden")
}

func TestATTLOfZeroMeansNothingExpires(t *testing.T) {
	useTempCache(t)
	viper.Set("cache.ttl_seconds", 0)

	require.NoError(t, Set(Entry{
		Key:       "ancient",
		Response:  "still good",
		CreatedAt: time.Now().Add(-10000 * time.Hour),
	}))

	entry, found, err := Get("ancient")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "still good", entry.Response)
}

func TestTTLReadsSecondsAndTreatsNegativesAsOff(t *testing.T) {
	useTempCache(t)

	viper.Set("cache.ttl_seconds", 90)
	require.Equal(t, 90*time.Second, TTL())

	viper.Set("cache.ttl_seconds", -1)
	require.Equal(t, time.Duration(0), TTL(), "a negative TTL disables expiry rather than expiring everything")
}

func TestEnabledFollowsConfiguration(t *testing.T) {
	useTempCache(t)

	require.False(t, Enabled(), "the cache is off unless it was asked for")
	viper.Set("cache.enabled", true)
	require.True(t, Enabled())
}

func TestListReportsStoredEntriesAndSkipsForeignFiles(t *testing.T) {
	dir := useTempCache(t)

	require.NoError(t, Set(Entry{Key: "one", Label: "first", PluginID: "ask", Model: "m"}))
	require.NoError(t, Set(Entry{Key: "two", Label: "second", PluginID: "chat", Model: "m"}))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not mine"), 0o600))

	entries, err := List()
	require.NoError(t, err)
	require.Len(t, entries, 2)

	labels := map[string]bool{}
	for _, e := range entries {
		labels[e.Label] = true
		require.Positive(t, e.SizeBytes)
	}
	require.Equal(t, map[string]bool{"first": true, "second": true}, labels)
}

func TestListOnAnAbsentDirectoryIsEmptyRatherThanAnError(t *testing.T) {
	useTempCache(t)

	entries, err := List()
	require.NoError(t, err)
	require.Empty(t, entries)
}

// A cache directory that is a file is a mistake worth naming.
func TestListRefusesACacheDirectoryThatIsAFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, os.WriteFile(dir, []byte("in the way"), 0o600))
	viper.Reset()
	viper.Set("cache.dir", dir)
	t.Cleanup(viper.Reset)

	_, err := List()
	require.ErrorContains(t, err, "is not a directory")
}

func TestListRefusesAnEntryItCannotRead(t *testing.T) {
	dir := useTempCache(t)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0o600))

	_, err := List()
	require.Error(t, err)
}

func TestStatsCountsEntriesAndTheirSize(t *testing.T) {
	useTempCache(t)

	require.NoError(t, Set(Entry{Key: "one", Response: "a"}))
	require.NoError(t, Set(Entry{Key: "two", Response: "b"}))

	stats, err := StatsInfo()
	require.NoError(t, err)
	require.Equal(t, 2, stats.Count)
	require.Positive(t, stats.SizeBytes)
}

func TestStatsOnAnAbsentDirectoryIsZero(t *testing.T) {
	useTempCache(t)

	stats, err := StatsInfo()
	require.NoError(t, err)
	require.Equal(t, Stats{}, stats)
}

func TestStatsIgnoresExpiredEntries(t *testing.T) {
	useTempCache(t)
	viper.Set("cache.ttl_seconds", 1)

	require.NoError(t, Set(Entry{Key: "fresh", Response: "a", CreatedAt: time.Now()}))
	require.NoError(t, Set(Entry{Key: "stale", Response: "b", CreatedAt: time.Now().Add(-time.Hour)}))

	stats, err := StatsInfo()
	require.NoError(t, err)
	require.Equal(t, 1, stats.Count)
}

func TestClearAllRemovesEveryEntryAndReportsHowMany(t *testing.T) {
	dir := useTempCache(t)

	require.NoError(t, Set(Entry{Key: "one"}))
	require.NoError(t, Set(Entry{Key: "two"}))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("mine"), 0o600))

	removed, err := ClearAll()
	require.NoError(t, err)
	require.Equal(t, 2, removed)

	entries, err := List()
	require.NoError(t, err)
	require.Empty(t, entries)

	_, statErr := os.Stat(filepath.Join(dir, "keep.txt"))
	require.NoError(t, statErr, "ClearAll owns the entries it wrote, not the directory")
}

func TestClearAllOnAnAbsentDirectoryRemovesNothing(t *testing.T) {
	useTempCache(t)

	removed, err := ClearAll()
	require.NoError(t, err)
	require.Zero(t, removed)
}

func TestDeleteRemovesOneEntryAndForgivesAMissingOne(t *testing.T) {
	useTempCache(t)

	require.NoError(t, Set(Entry{Key: "one"}))
	require.NoError(t, Delete("one"))

	_, found, err := Get("one")
	require.NoError(t, err)
	require.False(t, found)

	require.NoError(t, Delete("one"), "deleting what is already gone is what the caller wanted")
}

func TestDeleteRefusesAnEmptyKey(t *testing.T) {
	useTempCache(t)

	require.Error(t, Delete(""))
}

// The entry on disk is JSON a person may need to read while debugging a wrong answer.
func TestAStoredEntryIsReadableJSON(t *testing.T) {
	dir := useTempCache(t)

	require.NoError(t, Set(Entry{
		Key:      "abc",
		PluginID: "ask",
		Model:    "llama3.1",
		Messages: []Message{{Role: "user", Content: "hello"}},
		Response: "hi",
	}))

	data, err := os.ReadFile(filepath.Join(dir, "abc.json"))
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Equal(t, "ask", raw["plugin_id"])
	require.Equal(t, "llama3.1", raw["model"])
	require.Equal(t, "hi", raw["response"])
}
