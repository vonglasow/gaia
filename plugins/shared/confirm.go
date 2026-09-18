package shared

import "io"

// ConfirmWith is nil with no terminal, so refusals say "nobody to ask".
func ConfirmWith(in io.Reader, out io.Writer) func(string) (bool, error) {
	if !HasTTYStdin() || !HasTTYStdout() {
		return nil
	}
	return func(message string) (bool, error) {
		return RunConfirmationPromptTUI(message, "Confirm", in, out)
	}
}
