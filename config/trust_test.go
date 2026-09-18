package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"gaia/config"
	"gaia/plugins/shared"
)

// The trust store decides whether a repository's own .gaia.yaml is merged.

func aHomeWithNoTrustStore(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

// normalized is what the store actually records.
func normalized(t *testing.T, path string) string {
	t.Helper()
	out, err := shared.Normalize(path)
	require.NoError(t, err)
	return out
}

func TestNothingIsTrustedOnAFreshMachine(t *testing.T) {
	aHomeWithNoTrustStore(t)
	repo := t.TempDir()

	trusted, err := config.IsRepositoryTrusted(repo)

	require.NoError(t, err, "an absent store is an empty store, not a failure")
	require.False(t, trusted)
}

func TestListingOnAFreshMachineIsEmptyRatherThanNil(t *testing.T) {
	aHomeWithNoTrustStore(t)

	repos, err := config.ListTrustedRepositories()

	require.NoError(t, err)
	require.Empty(t, repos)
}

func TestTrustingARepositoryIsRememberedAcrossCalls(t *testing.T) {
	aHomeWithNoTrustStore(t)
	repo := t.TempDir()

	require.NoError(t, config.TrustRepository(repo))

	trusted, err := config.IsRepositoryTrusted(repo)
	require.NoError(t, err)
	require.True(t, trusted)
}

func TestTrustingTwiceLeavesOneEntry(t *testing.T) {
	aHomeWithNoTrustStore(t)
	repo := t.TempDir()

	require.NoError(t, config.TrustRepository(repo))
	require.NoError(t, config.TrustRepository(repo))

	repos, err := config.ListTrustedRepositories()
	require.NoError(t, err)
	require.Len(t, repos, 1)
}

// Untrusting is the control a person reaches for after the fact.
func TestUntrustingTakesTheTrustBack(t *testing.T) {
	aHomeWithNoTrustStore(t)
	repo := t.TempDir()
	require.NoError(t, config.TrustRepository(repo))

	require.NoError(t, config.UntrustRepository(repo))

	trusted, err := config.IsRepositoryTrusted(repo)
	require.NoError(t, err)
	require.False(t, trusted)

	repos, err := config.ListTrustedRepositories()
	require.NoError(t, err)
	require.Empty(t, repos)
}

func TestUntrustingWhatWasNeverTrustedIsNotAnError(t *testing.T) {
	aHomeWithNoTrustStore(t)

	require.NoError(t, config.UntrustRepository(t.TempDir()))
}

func TestTrustIsPerRepositoryAndNotShared(t *testing.T) {
	aHomeWithNoTrustStore(t)
	trustedRepo, otherRepo := t.TempDir(), t.TempDir()
	require.NoError(t, config.TrustRepository(trustedRepo))

	trusted, err := config.IsRepositoryTrusted(otherRepo)
	require.NoError(t, err)
	require.False(t, trusted, "trusting one repository must not trust the next one cloned")
}

func TestListingReportsEveryTrustedRootSorted(t *testing.T) {
	aHomeWithNoTrustStore(t)
	first, second := t.TempDir(), t.TempDir()
	require.NoError(t, config.TrustRepository(first))
	require.NoError(t, config.TrustRepository(second))

	repos, err := config.ListTrustedRepositories()

	require.NoError(t, err)
	require.Len(t, repos, 2)
	require.Contains(t, repos, normalized(t, first))
	require.Contains(t, repos, normalized(t, second))
	require.IsIncreasing(t, repos, "the listing is sorted, so two runs are diffable")
}

// The store is rewritten through a temporary file and a rename.
func TestTheStoreOnDiskIsReadableBack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := t.TempDir()
	require.NoError(t, config.TrustRepository(repo))

	entries, err := os.ReadDir(filepath.Join(home, ".config", "gaia"))
	require.NoError(t, err)

	var store string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			store = filepath.Join(home, ".config", "gaia", e.Name())
		}
	}
	require.NotEmpty(t, store, "trusting wrote a store")

	data, err := os.ReadFile(store)
	require.NoError(t, err)
	require.Contains(t, string(data), "trusted_repos")
	require.Contains(t, string(data), normalized(t, repo))
}

func TestNoTemporaryFileIsLeftBehind(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, config.TrustRepository(t.TempDir()))

	entries, err := os.ReadDir(filepath.Join(home, ".config", "gaia"))
	require.NoError(t, err)
	for _, e := range entries {
		require.NotContains(t, e.Name(), "trusted-repos-",
			"the rename replaced the store; the temporary file is not part of the result")
	}
}

// --- resolving which root is being trusted --------------------------------

func TestResolvingARootFindsTheGitRepositoryAPathSitsIn(t *testing.T) {
	aHomeWithNoTrustStore(t)
	repo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
	nested := filepath.Join(repo, "plugins", "ask")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	root, err := config.ResolveRepositoryRootFromPath(nested)

	require.NoError(t, err)
	require.Equal(t, normalized(t, repo), root,
		"trust is granted to the repository, not to whichever subdirectory happened to be current")
}

func TestResolvingARootFromAFileUsesItsDirectory(t *testing.T) {
	aHomeWithNoTrustStore(t)
	repo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
	file := filepath.Join(repo, ".gaia.yaml")
	require.NoError(t, os.WriteFile(file, []byte("ask:\n  model: x\n"), 0o600))

	root, err := config.ResolveRepositoryRootFromPath(file)

	require.NoError(t, err)
	require.Equal(t, normalized(t, repo), root)
}

// Outside a git repository there is no root to walk up to.
func TestResolvingARootOutsideGitKeepsTheDirectory(t *testing.T) {
	aHomeWithNoTrustStore(t)
	dir := t.TempDir()

	root, err := config.ResolveRepositoryRootFromPath(dir)

	require.NoError(t, err)
	require.Equal(t, normalized(t, dir), root)
}

func TestResolvingAnEmptyPathMeansHere(t *testing.T) {
	aHomeWithNoTrustStore(t)

	root, err := config.ResolveRepositoryRootFromPath("   ")

	require.NoError(t, err)
	require.NotEmpty(t, root)
}

func TestResolvingAPathThatIsNotThereFails(t *testing.T) {
	aHomeWithNoTrustStore(t)

	_, err := config.ResolveRepositoryRootFromPath(filepath.Join(t.TempDir(), "no-such-place"))

	require.Error(t, err, "trusting a path nobody can name is not something to do quietly")
}

// Trusting a subdirectory and then asking about the repository root must not answer yes.
func TestTrustIsNotInheritedByAParentDirectory(t *testing.T) {
	aHomeWithNoTrustStore(t)
	parent := t.TempDir()
	child := filepath.Join(parent, "nested")
	require.NoError(t, os.MkdirAll(child, 0o755))
	require.NoError(t, config.TrustRepository(child))

	trusted, err := config.IsRepositoryTrusted(parent)

	require.NoError(t, err)
	require.False(t, trusted)
}
