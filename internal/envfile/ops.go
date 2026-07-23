package envfile

import "strings"

// Get returns the decoded value of key. Duplicate keys follow environment
// semantics: the last occurrence wins.
func (f *File) Get(key string) (string, bool) {
	for i := len(f.Lines) - 1; i >= 0; i-- {
		if f.Lines[i].Kind == Entry && f.Lines[i].Key == key {
			return f.Lines[i].Value, true
		}
	}
	return "", false
}

// Set updates the last occurrence of key in place — preserving its quoting
// style, spacing, and inline comment — or appends a new entry.
func (f *File) Set(key, value string) {
	for i := len(f.Lines) - 1; i >= 0; i-- {
		if f.Lines[i].Kind == Entry && f.Lines[i].Key == key {
			setValue(&f.Lines[i], value)
			return
		}
	}
	f.appendEntry(key, value)
}

// Unset removes every occurrence of key and reports whether any existed.
func (f *File) Unset(key string) bool {
	removed := false
	kept := f.Lines[:0]
	for _, ln := range f.Lines {
		if ln.Kind == Entry && ln.Key == key {
			removed = true
			continue
		}
		kept = append(kept, ln)
	}
	f.Lines = kept
	return removed
}

// Keys returns unique keys in order of first occurrence.
func (f *File) Keys() []string {
	seen := make(map[string]bool)
	var keys []string
	for _, ln := range f.Lines {
		if ln.Kind == Entry && !seen[ln.Key] {
			seen[ln.Key] = true
			keys = append(keys, ln.Key)
		}
	}
	return keys
}

// MalformedLines returns the 1-based physical line numbers that failed to
// parse. Push refuses files where this is non-empty.
func (f *File) MalformedLines() []int {
	var nos []int
	for _, ln := range f.Lines {
		if ln.Kind == Malformed {
			nos = append(nos, ln.StartLine)
		}
	}
	return nos
}

func setValue(ln *Line, value string) {
	q := ln.Quote
	if q == 0 && needsQuoting(value) {
		q = '"'
	}
	if q == '\'' && strings.ContainsRune(value, '\'') {
		q = '"'
	}
	switch q {
	case 0:
		ln.RawValue = value
	case '\'':
		ln.RawValue = "'" + value + "'"
	default:
		ln.RawValue = `"` + encodeDouble(value) + `"`
	}
	ln.Quote = q
	ln.Value = value
	ln.Raw = ln.Prefix + ln.ExportTok + ln.Key + ln.Sep + ln.RawValue + ln.Suffix
}

func (f *File) appendEntry(key, value string) {
	term := f.dominantTerm()
	if n := len(f.Lines); n > 0 && f.Lines[n-1].Term == "" {
		f.Lines[n-1].Term = term
	}
	ln := Line{Kind: Entry, Key: key, Sep: "=", Term: term}
	setValue(&ln, value)
	f.Lines = append(f.Lines, ln)
}

func (f *File) dominantTerm() string {
	for i := len(f.Lines) - 1; i >= 0; i-- {
		if t := f.Lines[i].Term; t != "" {
			return t
		}
	}
	return "\n"
}

func needsQuoting(v string) bool {
	return strings.ContainsAny(v, " \t\n\r#'\"\\")
}
