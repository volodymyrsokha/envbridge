// Package envfile parses and rewrites dotenv files losslessly. Parsing then
// rendering any input — valid or not — yields byte-identical output;
// mutations rebuild only the affected entry's bytes. Lines that don't parse
// are kept verbatim as Malformed so callers can refuse to push them without
// ever corrupting the file.
package envfile

type Kind int

const (
	Blank Kind = iota
	Comment
	Entry
	Malformed
)

// Line is one logical line. A quoted value may span several physical lines,
// in which case Raw contains the inner terminators and StartLine points at
// the first physical line.
type Line struct {
	Kind      Kind
	StartLine int
	Raw       string
	Term      string

	Prefix    string
	ExportTok string
	Key       string
	Sep       string
	Quote     byte
	RawValue  string
	Value     string
	Suffix    string
}

type File struct {
	Lines []Line
}
