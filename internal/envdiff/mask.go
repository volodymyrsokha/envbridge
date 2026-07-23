package envdiff

// Mask hides a secret value for display. Long values keep their last four
// characters as a recognizable fingerprint; short values are fully hidden so
// the mask never reveals most of the secret. The mask length is fixed to
// avoid leaking the value's length.
func Mask(v string) string {
	if v == "" {
		return "(empty)"
	}
	runes := []rune(v)
	if len(runes) >= 12 {
		return "••••" + string(runes[len(runes)-4:])
	}
	return "••••••••"
}
