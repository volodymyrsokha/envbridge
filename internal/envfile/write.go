package envfile

import "strings"

func (f *File) Render() []byte {
	var b strings.Builder
	for _, ln := range f.Lines {
		b.WriteString(ln.Raw)
		b.WriteString(ln.Term)
	}
	return []byte(b.String())
}
