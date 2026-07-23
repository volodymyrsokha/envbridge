package envfile

import "strings"

type phys struct {
	text string
	term string
}

func splitPhys(s string) []phys {
	var out []phys
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '\n' {
			continue
		}
		end, term := i, "\n"
		if end > start && s[end-1] == '\r' {
			end--
			term = "\r\n"
		}
		out = append(out, phys{s[start:end], term})
		start = i + 1
	}
	if start < len(s) {
		out = append(out, phys{s[start:], ""})
	}
	return out
}

// Parse is total: any input produces a File whose Render() reproduces the
// input byte for byte. Unparseable content becomes Malformed lines.
func Parse(data []byte) *File {
	lines := splitPhys(string(data))
	f := &File{}
	for i := 0; i < len(lines); {
		ln, consumed := parseAt(lines, i)
		f.Lines = append(f.Lines, ln)
		i += consumed
	}
	return f
}

func parseAt(lines []phys, i int) (Line, int) {
	text := lines[i].text
	term := lines[i].term
	no := i + 1

	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Line{Kind: Blank, StartLine: no, Raw: text, Term: term}, 1
	}
	if trimmed[0] == '#' {
		return Line{Kind: Comment, StartLine: no, Raw: text, Term: term}, 1
	}

	j := skipWS(text, 0)
	prefix := text[:j]

	exportTok := ""
	if strings.HasPrefix(text[j:], "export") {
		k := skipWS(text, j+len("export"))
		if k > j+len("export") && k < len(text) && isKeyStart(text[k]) {
			exportTok = text[j:k]
			j = k
		}
	}

	ks := j
	for j < len(text) && isKeyChar(text[j]) {
		j++
	}
	key := text[ks:j]
	if key == "" || !isKeyStart(key[0]) {
		return malformed(lines, i)
	}

	eq := skipWS(text, j)
	if eq >= len(text) || text[eq] != '=' {
		return malformed(lines, i)
	}
	ve := skipWS(text, eq+1)
	sep := text[j:ve]
	wsAfterEq := ve > eq+1
	rest := text[ve:]

	ln := Line{
		Kind:      Entry,
		StartLine: no,
		Term:      term,
		Prefix:    prefix,
		ExportTok: exportTok,
		Key:       key,
		Sep:       sep,
	}

	switch {
	case rest == "":
		// KEY= with nothing after
	case rest[0] == '"' || rest[0] == '\'':
		quote := rest[0]
		combined, closeIdx, lastIdx := scanQuoted(lines, i, rest, quote)
		if closeIdx < 0 {
			return malformed(lines, i)
		}
		after := combined[closeIdx+1:]
		if a := skipWS(after, 0); a < len(after) && after[a] != '#' {
			return malformed(lines, i)
		}
		ln.Quote = quote
		ln.RawValue = combined[:closeIdx+1]
		ln.Suffix = after
		inner := combined[1:closeIdx]
		if quote == '"' {
			ln.Value = decodeDouble(inner)
		} else {
			ln.Value = inner
		}
		ln.Raw = text[:ve] + combined
		ln.Term = lines[lastIdx].term
		return ln, lastIdx - i + 1
	case rest[0] == '#' && wsAfterEq:
		ln.Suffix = rest
	default:
		ci := -1
		for k := 1; k < len(rest); k++ {
			if rest[k] == '#' && (rest[k-1] == ' ' || rest[k-1] == '\t') {
				ci = k
				break
			}
		}
		end := len(rest)
		if ci >= 0 {
			end = ci
		}
		val := strings.TrimRight(rest[:end], " \t")
		ln.RawValue = val
		ln.Value = val
		ln.Suffix = rest[len(val):]
	}

	ln.Raw = text
	return ln, 1
}

// scanQuoted finds the closing quote, consuming continuation lines for
// multiline values. Returns the accumulated text starting at the opening
// quote, the index of the closing quote within it (-1 if unterminated), and
// the index of the last physical line consumed.
func scanQuoted(lines []phys, i int, rest string, quote byte) (string, int, int) {
	combined := rest
	lineIdx := i
	m := 1
	esc := false
	for {
		for m < len(combined) {
			c := combined[m]
			if esc {
				esc = false
				m++
				continue
			}
			if quote == '"' && c == '\\' {
				esc = true
				m++
				continue
			}
			if c == quote {
				return combined, m, lineIdx
			}
			m++
		}
		if lineIdx+1 >= len(lines) {
			return combined, -1, lineIdx
		}
		combined += lines[lineIdx].term + lines[lineIdx+1].text
		lineIdx++
	}
}

func malformed(lines []phys, i int) (Line, int) {
	return Line{Kind: Malformed, StartLine: i + 1, Raw: lines[i].text, Term: lines[i].term}, 1
}

func skipWS(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}

func isKeyStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isKeyChar(c byte) bool {
	return isKeyStart(c) || c == '.' || (c >= '0' && c <= '9')
}

func decodeDouble(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 == len(s) {
			b.WriteByte(c)
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '"':
			b.WriteByte('"')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func encodeDouble(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
