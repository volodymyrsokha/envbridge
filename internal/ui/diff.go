package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/volodymyrsokha/envbridge/internal/envdiff"
)

var (
	addStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	removeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	changeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

// RenderDiff formats a semantic env diff, masking values unless showValues
// is set.
func RenderDiff(changes []envdiff.Change, showValues bool) string {
	if len(changes) == 0 {
		return "  no changes"
	}
	display := func(v string) string {
		if showValues {
			return v
		}
		return envdiff.Mask(v)
	}
	keyWidth := 0
	for _, c := range changes {
		if len(c.Key) > keyWidth {
			keyWidth = len(c.Key)
		}
	}
	var b strings.Builder
	for _, c := range changes {
		switch c.Kind {
		case envdiff.Added:
			line := fmt.Sprintf("  + %-*s  %s", keyWidth, c.Key, display(c.New))
			b.WriteString(styleLine(line, addStyle))
		case envdiff.Removed:
			line := fmt.Sprintf("  - %-*s", keyWidth, c.Key)
			b.WriteString(styleLine(line, removeStyle))
		case envdiff.Changed:
			line := fmt.Sprintf("  ~ %-*s  %s → %s", keyWidth, c.Key, display(c.Old), display(c.New))
			b.WriteString(styleLine(line, changeStyle))
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func styleLine(s string, style lipgloss.Style) string {
	if colorOff {
		return s
	}
	return style.Render(s)
}
