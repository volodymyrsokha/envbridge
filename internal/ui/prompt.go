package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Confirm asks a yes/no question on the terminal, defaulting to no.
func Confirm(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
