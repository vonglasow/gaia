package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gaia/internal/filelock"
	"gaia/internal/pathutil"
	"gaia/internal/termio"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const (
	localConfigFileName = ".gaia.yaml"
	trustStoreFileName  = "trusted-repos.yaml"
	trustLockTimeout    = 5 * time.Second
)

func loadTrustedLocalConfig() error {
	localConfigPath, repoRoot, found, err := discoverLocalConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to check local config: %v\n", err)
		return nil
	}
	if !found {
		return nil
	}

	trusted, err := IsRepositoryTrusted(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to read trusted repositories: %v\n", err)
		trusted = false
	}

	if !trusted {
		if !termio.HasTTYStdin() || !termio.HasTTYStdout() {
			fmt.Fprintf(os.Stderr, "Warning: ignored untrusted local config %s (run in a TTY to trust this repository)\n", localConfigPath)
			return nil
		}

		allow, promptErr := promptTrustRepository(repoRoot, localConfigPath)
		if promptErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to read trust confirmation: %v\n", promptErr)
			return nil
		}
		if !allow {
			fmt.Fprintf(os.Stderr, "Info: local config not trusted, using user/global config only\n")
			return nil
		}

		if trustErr := TrustRepository(repoRoot); trustErr != nil {
			return fmt.Errorf("trust repository: %w", trustErr)
		}
	}

	if err := mergeLocalConfig(localConfigPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load local config %s: %v\n", localConfigPath, err)
		return nil
	}

	return nil
}

func promptTrustRepository(repoRoot, localConfigPath string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stderr, "Detected local Gaia config at %s\n", localConfigPath)
	fmt.Fprintf(os.Stderr, "Trust repository %s and load local overrides? [y/N]: ", repoRoot)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	input = strings.ToLower(strings.TrimSpace(input))
	return input == "y" || input == "yes", nil
}

func mergeLocalConfig(localConfigPath string) error {
	local := viper.New()
	local.SetConfigFile(localConfigPath)
	local.SetConfigType("yaml")
	if err := local.ReadInConfig(); err != nil {
		return err
	}
	return viper.MergeConfigMap(local.AllSettings())
}

// IsRepositoryTrusted returns whether the repository root is trusted for local .gaia.yaml overrides.
func IsRepositoryTrusted(repoRoot string) (bool, error) {
	normalized, err := pathutil.Normalize(repoRoot)
	if err != nil {
		return false, err
	}

	trusted := false
	err = withTrustStoreReadLock(func(path string) error {
		repos, err := readTrustedReposFromPath(path)
		if err != nil {
			return err
		}
		trusted = repos[normalized]
		return nil
	})
	if err != nil {
		return false, err
	}

	return trusted, nil
}

// TrustRepository persists trust for the given repository root in the user trust store.
func TrustRepository(repoRoot string) error {
	normalized, err := pathutil.Normalize(repoRoot)
	if err != nil {
		return err
	}

	return withTrustStoreWriteLock(func(path string) error {
		repos, err := readTrustedReposFromPath(path)
		if err != nil {
			return err
		}
		repos[normalized] = true
		return writeTrustedReposToPath(path, repos)
	})
}

// UntrustRepository removes trust for the given repository root.
func UntrustRepository(repoRoot string) error {
	normalized, err := pathutil.Normalize(repoRoot)
	if err != nil {
		return err
	}

	return withTrustStoreWriteLock(func(path string) error {
		repos, err := readTrustedReposFromPath(path)
		if err != nil {
			return err
		}
		delete(repos, normalized)
		return writeTrustedReposToPath(path, repos)
	})
}

// ListTrustedRepositories returns all trusted repository roots.
func ListTrustedRepositories() ([]string, error) {
	trusted := []string{}
	err := withTrustStoreReadLock(func(path string) error {
		repos, err := readTrustedReposFromPath(path)
		if err != nil {
			return err
		}
		for root, ok := range repos {
			if ok {
				trusted = append(trusted, root)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(trusted)
	return trusted, nil
}

// ResolveRepositoryRootFromPath resolves a repository root for trust operations.
// If path is inside a git repository, it returns the git root.
// Otherwise, it returns the normalized absolute directory path.
func ResolveRepositoryRootFromPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	start := abs
	if !info.IsDir() {
		start = filepath.Dir(abs)
	}
	normalizedStart, err := pathutil.Normalize(start)
	if err != nil {
		return "", err
	}
	if gitRoot, ok, err := findGitRoot(normalizedStart); err == nil && ok {
		return gitRoot, nil
	}
	return normalizedStart, nil
}

func discoverLocalConfig() (localConfigPath string, repoRoot string, found bool, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", false, err
	}
	cwd, err = pathutil.Normalize(cwd)
	if err != nil {
		return "", "", false, err
	}

	repoRoot = cwd
	if gitRoot, ok, err := findGitRoot(cwd); err == nil && ok {
		repoRoot = gitRoot
	}

	dir := cwd
	for {
		candidate := filepath.Join(dir, localConfigFileName)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, repoRoot, true, nil
		}
		if dir == repoRoot {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", repoRoot, false, nil
}

func findGitRoot(start string) (string, bool, error) {
	dir := start
	for {
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			root, normErr := pathutil.Normalize(dir)
			if normErr != nil {
				return "", false, normErr
			}
			return root, true, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}

func withTrustStoreReadLock(fn func(path string) error) error {
	return withTrustStoreLock(filelock.Shared, fn)
}

func withTrustStoreWriteLock(fn func(path string) error) error {
	return withTrustStoreLock(filelock.Exclusive, fn)
}

func withTrustStoreLock(mode filelock.Mode, fn func(path string) error) error {
	storePath, err := trustStorePath()
	if err != nil {
		return err
	}
	lockPath := storePath + ".lock"
	if err := filelock.With(lockPath, mode, trustLockTimeout, func() error {
		return fn(storePath)
	}); err != nil {
		return fmt.Errorf("lock trust store: %w", err)
	}
	return nil
}

func readTrustedReposFromPath(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, fs.ErrNotExist) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("read trusted repos: %w", err)
	}

	var payload struct {
		TrustedRepos map[string]bool `yaml:"trusted_repos"`
	}
	if err := yaml.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode trusted repos: %w", err)
	}

	repos := make(map[string]bool, len(payload.TrustedRepos))
	for key, isTrusted := range payload.TrustedRepos {
		normalized, normErr := pathutil.Normalize(key)
		if normErr != nil {
			continue
		}
		repos[normalized] = isTrusted
	}
	return repos, nil
}

func writeTrustedReposToPath(path string, repos map[string]bool) error {
	payload := struct {
		TrustedRepos map[string]bool `yaml:"trusted_repos"`
	}{
		TrustedRepos: repos,
	}

	data, err := yaml.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode trusted repos: %w", err)
	}

	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, "trusted-repos-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp trusted repo file: %w", err)
	}
	tmpPath := tmpFile.Name()

	writeErr := error(nil)
	if _, err := tmpFile.Write(data); err != nil {
		writeErr = fmt.Errorf("write temp trusted repo file: %w", err)
	}
	if err := tmpFile.Chmod(0o600); err != nil && writeErr == nil {
		writeErr = fmt.Errorf("chmod temp trusted repo file: %w", err)
	}
	if err := tmpFile.Close(); err != nil && writeErr == nil {
		writeErr = fmt.Errorf("close temp trusted repo file: %w", err)
	}
	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return writeErr
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace trusted repo file: %w", err)
	}

	return nil
}

func trustStorePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}
	configDir := filepath.Join(homeDir, ".config", "gaia")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create config directory %s: %w", configDir, err)
	}
	return filepath.Join(configDir, trustStoreFileName), nil
}
