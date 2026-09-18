// Package bdd holds gaia's acceptance scenarios.
package bdd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/spf13/viper"

	"gaia/config"
	"gaia/kernel"
	"gaia/plugins"
	"gaia/plugins/ask"
	"gaia/plugins/shared"
)

// result is what one run of a command line produced.
type result struct {
	stdout string
	stderr string
	code   int
}

// world carries what one scenario needs between its steps.
type world struct {
	t    *testing.T
	home string
	cfg  string
	out  result
}

// newWorld remembers the test the scenarios of one feature run under.
//
// Nothing here may reach a model, and that now includes asking Ollama what it
// holds: a scenario must read the same on a machine with models and without.
func newWorld(t *testing.T) *world {
	ask.NoModelsInstalled(t)
	return &world{t: t}
}

// reset gives the next scenario a home nobody else has written to.
func (w *world) reset() {
	w.home = w.t.TempDir()
	w.t.Setenv("HOME", w.home)
	// Viper reads this before it reads $HOME.
	w.t.Setenv("GAIA_CONFIG", "")
	w.cfg = ""
	w.out = result{}
}

// run executes one command line against a freshly built kernel.
func (w *world) run(line string) result {
	viper.Reset()
	config.CfgFile = ""

	args := splitArgs(line)
	if w.cfg != "" {
		args = append([]string{"--config", w.cfg}, args...)
	}

	var stdout, stderr bytes.Buffer
	k := kernel.NewKernel()
	if err := plugins.RegisterAll(k); err != nil {
		fmt.Fprintln(&stderr, err)
		return result{stderr: stderr.String(), code: 1}
	}
	k.RootCmd.SetOut(&stdout)
	k.RootCmd.SetErr(&stderr)

	code := 0
	if err := k.Execute(args); err != nil {
		// Mirrors main.go exactly, ErrReported included.
		if !shared.WasReported(err) {
			fmt.Fprintln(&stderr, err)
		}
		code = 1
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// splitArgs turns the command line a scenario wrote into the arguments a shell would.
func splitArgs(line string) []string {
	var (
		args    []string
		current strings.Builder
		quoted  bool
		started bool
	)
	for _, r := range line {
		if r == '"' {
			quoted = !quoted
			started = true
			continue
		}
		if r == ' ' && !quoted {
			if started {
				args = append(args, current.String())
				current.Reset()
				started = false
			}
			continue
		}
		current.WriteRune(r)
		started = true
	}
	if started {
		args = append(args, current.String())
	}
	return args
}

// --- steps shared by every feature ----------------------------------------

func (w *world) aHomeOfMyOwn() error {
	if w.home == "" {
		return fmt.Errorf("no home was prepared")
	}
	return nil
}

// aConfigurationFileContaining writes the YAML elsewhere, which also proves --config.
func (w *world) aConfigurationFileContaining(body *godog.DocString) error {
	path := filepath.Join(w.home, "scenario-config.yaml")
	if err := os.WriteFile(path, []byte(body.Content), 0o600); err != nil {
		return err
	}
	w.cfg = path
	return nil
}

// aRolesDirectoryHolding writes one role file into a directory of the scenario's own.
func (w *world) aRolesDirectoryHolding(name string, body *godog.DocString) error {
	dir := filepath.Join(w.home, "roles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body.Content), 0o600); err != nil {
		return err
	}
	path := filepath.Join(w.home, "roles-config.yaml")
	if err := os.WriteFile(path, []byte("roles:\n  directory: "+dir+"\n"), 0o600); err != nil {
		return err
	}
	w.cfg = path
	return nil
}

func (w *world) iRun(line string) error {
	w.out = w.run(line)
	return nil
}

func (w *world) itExitsWithCode(want int) error {
	if w.out.code != want {
		return fmt.Errorf("exited with %d, wanted %d\nstdout: %s\nstderr: %s",
			w.out.code, want, w.out.stdout, w.out.stderr)
	}
	return nil
}

func (w *world) theOutputContains(want string) error {
	if !strings.Contains(w.out.stdout, want) {
		return fmt.Errorf("standard output does not contain %q\ngot: %s", want, w.out.stdout)
	}
	return nil
}

func (w *world) theOutputDoesNotContain(unwanted string) error {
	if strings.Contains(w.out.stdout, unwanted) {
		return fmt.Errorf("standard output contains %q and should not\ngot: %s", unwanted, w.out.stdout)
	}
	return nil
}

func (w *world) standardErrorContains(want string) error {
	if !strings.Contains(w.out.stderr, want) {
		return fmt.Errorf("standard error does not contain %q\ngot: %s", want, w.out.stderr)
	}
	return nil
}

func (w *world) standardErrorDoesNotContain(unwanted string) error {
	if strings.Contains(w.out.stderr, unwanted) {
		return fmt.Errorf("standard error contains %q and should not\ngot: %s", unwanted, w.out.stderr)
	}
	return nil
}

func (w *world) nothingIsWrittenToStandardError() error {
	if strings.TrimSpace(w.out.stderr) != "" {
		return fmt.Errorf("standard error was written to: %s", w.out.stderr)
	}
	return nil
}

// register wires the shared vocabulary.
func (w *world) register(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w.reset()
		return ctx, nil
	})
	sc.Step(`^a home of my own$`, w.aHomeOfMyOwn)
	sc.Step(`^a configuration file containing:$`, w.aConfigurationFileContaining)
	sc.Step(`^a roles directory holding "([^"]*)":$`, w.aRolesDirectoryHolding)
	sc.Step(`^I run "([^"]*)"$`, w.iRun)
	sc.Step(`^it exits with code (\d+)$`, w.itExitsWithCode)
	sc.Step(`^the output contains "([^"]*)"$`, w.theOutputContains)
	sc.Step(`^the output does not contain "([^"]*)"$`, w.theOutputDoesNotContain)
	sc.Step(`^standard error contains "([^"]*)"$`, w.standardErrorContains)
	sc.Step(`^standard error does not contain "([^"]*)"$`, w.standardErrorDoesNotContain)
	sc.Step(`^nothing is written to standard error$`, w.nothingIsWrittenToStandardError)
}

// suite describes the run of one feature file.
func suite(t *testing.T, feature string) godog.TestSuite {
	return godog.TestSuite{
		Name:                "gaia",
		ScenarioInitializer: newWorld(t).register,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{feature},
			Strict:   true,
			NoColors: true,
			TestingT: t,
		},
	}
}

// runFeature is what each feature's test calls.
func runFeature(t *testing.T, feature string) {
	t.Helper()
	if suite(t, feature).Run() != 0 {
		t.Fatalf("%s: scenarios failed", feature)
	}
}
