package commands

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// CompDirectiveAsChosen says the candidates were chosen by the program, not
// by the letters typed starting them: "ogi" offers 荻窪 (its reading,
// ogikubo). A shell keeps only the candidates that start with the word on the
// line, which would leave none; with this bit, the scripts mywant completion
// writes (PatchCompletionScripts) offer them as they are, and the one chosen
// replaces the word. A bit cobra does not use (its own end at 32).
const CompDirectiveAsChosen cobra.ShellCompDirective = 1 << 10

// PatchCompletionScripts makes `mywant completion zsh` and `bash` honour
// CompDirectiveAsChosen: cobra's own scripts, with that one step added.
func PatchCompletionScripts(root *cobra.Command) {
	root.InitDefaultCompletionCmd()
	var completion *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "completion" {
			completion = c
		}
	}
	if completion == nil {
		return
	}
	for _, sub := range completion.Commands() {
		switch sub.Name() {
		case "zsh":
			sub.RunE = func(cmd *cobra.Command, args []string) error {
				var buf bytes.Buffer
				noDesc, _ := cmd.Flags().GetBool("no-descriptions")
				var err error
				if noDesc {
					err = cmd.Root().GenZshCompletionNoDesc(&buf)
				} else {
					err = cmd.Root().GenZshCompletion(&buf)
				}
				if err != nil {
					return err
				}
				_, err = fmt.Fprint(cmd.OutOrStdout(), patchZsh(buf.String()))
				return err
			}
			sub.Run = nil
		case "bash":
			sub.RunE = func(cmd *cobra.Command, args []string) error {
				var buf bytes.Buffer
				noDesc, _ := cmd.Flags().GetBool("no-descriptions")
				if err := cmd.Root().GenBashCompletionV2(&buf, !noDesc); err != nil {
					return err
				}
				_, err := fmt.Fprint(cmd.OutOrStdout(), patchBash(buf.String()))
				return err
			}
			sub.Run = nil
		}
	}
}

// zsh: _describe hands its options to compadd, and -U takes the candidates
// without matching them against the word, which they then replace.
func patchZsh(script string) string {
	old := `if eval _describe $keepOrder "completions" completions $flagPrefix $noSpace; then`
	if !strings.Contains(script, old) {
		return script
	}
	return strings.Replace(script, old, fmt.Sprintf(`local asChosen=""
        if [ $((directive & %d)) -ne 0 ]; then
            # Chosen by the program (mywant: a name by its reading): offered as they are.
            asChosen="-U"
        fi
        if eval _describe $keepOrder "completions" completions $flagPrefix $noSpace $asChosen; then`, CompDirectiveAsChosen), 1)
}

// bash: the candidates, their descriptions taken off, are the reply as they are.
func patchBash(script string) string {
	fn := "_handle_standard_completion_case() {\n    local tab=$'\\t'\n"
	i := strings.Index(script, fn)
	if i < 0 {
		return script
	}
	at := i + len(fn)
	add := fmt.Sprintf(`
    # Chosen by the program (mywant: a name by its reading): offered as they are.
    if (( (directive & %d) != 0 )); then
        local comp
        COMPREPLY=()
        for comp in "${completions[@]}"; do COMPREPLY+=("${comp%%%%$tab*}"); done
        return 0
    fi
`, CompDirectiveAsChosen)
	return script[:at] + add + script[at:]
}
