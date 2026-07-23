// Package ui centralizes terminal rendering: styles, prompts, spinners, and
// error formatting. Nothing outside this package prints styled output.
package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorOff bool

	errorMark   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	successMark = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	emphasis    = lipgloss.NewStyle().Bold(true)
	hint        = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func DisableColor() { colorOff = true }

func RenderError(err error) string {
	if colorOff {
		return "error: " + err.Error()
	}
	return errorMark.Render("✗") + " " + err.Error()
}

func Success(s string) string {
	if colorOff {
		return "✓ " + s
	}
	return successMark.Render("✓") + " " + s
}

func Emphasize(s string) string {
	if colorOff {
		return s
	}
	return emphasis.Render(s)
}

func Hint(s string) string {
	if colorOff {
		return "  hint: " + s
	}
	return hint.Render("  hint: " + s)
}
