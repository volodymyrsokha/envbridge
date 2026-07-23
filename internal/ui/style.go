// Package ui centralizes terminal rendering: styles, prompts, spinners, and
// error formatting. Nothing outside this package prints styled output.
package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorOff bool

	errorMark = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	hint      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func DisableColor() { colorOff = true }

func RenderError(err error) string {
	if colorOff {
		return "error: " + err.Error()
	}
	return errorMark.Render("✗") + " " + err.Error()
}

func Hint(s string) string {
	if colorOff {
		return "  hint: " + s
	}
	return hint.Render("  hint: " + s)
}
