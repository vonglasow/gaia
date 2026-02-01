package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"gaia/api/operator"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// shellRunnerWithTimeout wraps ExecuteExternalCommandWithContext with a timeout.
type shellRunnerWithTimeout struct {
	timeout time.Duration
}

func (s *shellRunnerWithTimeout) Run(ctx context.Context, cmd string) (stdout, stderr string, err error) {
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}
	return ExecuteExternalCommandWithContext(ctx, cmd)
}

var InvestigateCmd = &cobra.Command{
	Use:   "investigate [goal]",
	Short: "Investigate a goal by running tools and reasoning (operator mode)",
	Long: `Run the operator to investigate a goal. The operator can run shell commands (e.g. df, du)
subject to safety checks. Use --dry-run to see what would be run without executing.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runInvestigate,
}

func init() {
	InvestigateCmd.Flags().IntP("max-steps", "n", 10, "Maximum number of operator steps")
	InvestigateCmd.Flags().Bool("dry-run", false, "Do not execute commands; only show what would be run")
	InvestigateCmd.Flags().BoolP("yes", "y", false, "Skip confirmation for medium-risk commands")
	InvestigateCmd.Flags().Bool("debug", false, "Print debug output (decisions and observations)")
}

func runInvestigate(cmd *cobra.Command, args []string) error {
	goal := args[0]
	maxSteps, _ := cmd.Flags().GetInt("max-steps")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	yes, _ := cmd.Flags().GetBool("yes")
	debug, _ := cmd.Flags().GetBool("debug")

	// Prefer flags; fallback to viper (e.g. GAIA_DEBUG=1)
	if !cmd.Flags().Lookup("debug").Changed {
		debug = viper.GetBool("debug")
	}

	timeoutSec := viper.GetInt("operator.command_timeout_seconds")
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	runner := &shellRunnerWithTimeout{timeout: time.Duration(timeoutSec) * time.Second}

	opts := operator.RunOptions{
		MaxSteps:          maxSteps,
		DryRun:            dryRun,
		Yes:               yes,
		Debug:             debug,
		Model:             viper.GetString("model"),
		Denylist:          getStringSlice("operator.denylist"),
		Allowlist:         getStringSlice("operator.allowlist"),
		ConfirmMediumRisk: viper.GetBool("operator.confirm_medium_risk"),
		ConfirmFunc:       promptForConfirmation,
		ShellRunner:       runner,
		MaxOutputBytes:    viper.GetInt("operator.output_max_bytes"),
	}

	ctx := context.Background()
	finalAnswer, err := operator.Run(ctx, goal, opts)
	if err != nil {
		if errors.Is(err, operator.ErrMaxStepsReached) {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		} else {
			return err
		}
	}
	fmt.Println(finalAnswer)
	return nil
}

func getStringSlice(key string) []string {
	v := viper.Get(key)
	if v == nil {
		return nil
	}
	if s, ok := v.([]string); ok {
		return s
	}
	if s, ok := v.([]interface{}); ok {
		out := make([]string, 0, len(s))
		for _, x := range s {
			if str, ok := x.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}
