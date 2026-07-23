package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// One shared reader: per-call bufio readers would swallow buffered lines
// between consecutive prompts when input is piped.
var stdin = bufio.NewReader(os.Stdin)

func readLine() string {
	line, _ := stdin.ReadString('\n')
	return strings.TrimSpace(line)
}

// Confirm asks a yes/no question on the terminal, defaulting to no.
func Confirm(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	answer := strings.ToLower(readLine())
	return answer == "y" || answer == "yes"
}

// Ask prompts for a line of input, returning def when the answer is empty.
func Ask(prompt, def string) string {
	if def != "" {
		fmt.Fprintf(os.Stderr, "? %s (%s) ", prompt, def)
	} else {
		fmt.Fprintf(os.Stderr, "? %s ", prompt)
	}
	if answer := readLine(); answer != "" {
		return answer
	}
	return def
}

// AskRequired re-prompts until it gets a non-empty answer.
func AskRequired(prompt string) string {
	for {
		if answer := Ask(prompt, ""); answer != "" {
			return answer
		}
		fmt.Fprintln(os.Stderr, hintText("  a value is required"))
	}
}

func hintText(s string) string {
	if colorOff {
		return s
	}
	return hint.Render(s)
}
