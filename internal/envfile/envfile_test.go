package envfile

import (
	"reflect"
	"testing"
)

var roundTripCases = []struct {
	name  string
	input string
}{
	{"empty", ""},
	{"simple", "KEY=value\n"},
	{"no trailing newline", "KEY=value"},
	{"crlf", "A=1\r\nB=2\r\n"},
	{"mixed terminators", "A=1\nB=2\r\nC=3"},
	{"blank lines", "\n\nA=1\n\n"},
	{"whitespace-only line", "  \t  \nA=1\n"},
	{"comments", "# header\nA=1\n  # indented comment\n"},
	{"inline comment", "A=1 # comment\n"},
	{"hash without space is value", "A=1#notacomment\n"},
	{"empty value", "A=\n"},
	{"empty value with comment", "A= # nothing\n"},
	{"export", "export A=1\n"},
	{"export with tabs", "export\tA=1\n"},
	{"key named export", "export=1\n"},
	{"export glued to key", "exportA=1\n"},
	{"spaces around equals", "A = 1\n"},
	{"leading whitespace", "  A=1\n"},
	{"single quoted", "A='hello world'\n"},
	{"double quoted", "A=\"hello world\"\n"},
	{"double quoted with escapes", `A="line1\nline2 \"quoted\" \\ end"` + "\n"},
	{"quoted with inline comment", "A=\"v\" # comment\n"},
	{"multiline double quoted", "A=\"line1\nline2\nline3\"\nB=2\n"},
	{"multiline single quoted", "A='line1\nline2'\n"},
	{"multiline crlf", "A=\"one\r\ntwo\"\r\n"},
	{"unclosed quote", "A=\"never closes\nB=2\n"},
	{"junk after closing quote", "A=\"v\" junk\n"},
	{"missing equals", "NOTANENTRY\n"},
	{"missing key", "=value\n"},
	{"invalid key start", "1A=x\n"},
	{"unicode value", "GREETING=привіт 👋\n"},
	{"dollar not expanded", "A=$HOME and ${OTHER}\n"},
	{"dotted key", "app.name=svc\n"},
	{"lone cr at eof", "A=1\r"},
	{"escaped quote spans lines", "A=\"a\\\nb\"\n"},
}

func TestRoundTrip(t *testing.T) {
	for _, tc := range roundTripCases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(Parse([]byte(tc.input)).Render())
			if got != tc.input {
				t.Errorf("round trip changed bytes\n in: %q\nout: %q", tc.input, got)
			}
		})
	}
}

func TestGet(t *testing.T) {
	cases := []struct {
		name  string
		input string
		key   string
		want  string
		ok    bool
	}{
		{"plain", "A=hello\n", "A", "hello", true},
		{"missing", "A=1\n", "B", "", false},
		{"trims unquoted", "A= padded  \n", "A", "padded", true},
		{"single quotes literal", `A='no \n escapes'` + "\n", "A", `no \n escapes`, true},
		{"double quote escapes", `A="tab\there\nnewline"` + "\n", "A", "tab\there\nnewline", true},
		{"unknown escape kept", `A="50\% off"` + "\n", "A", `50\% off`, true},
		{"last wins", "A=first\nA=second\n", "A", "second", true},
		{"inline comment excluded", "A=v # c\n", "A", "v", true},
		{"hash inside value", "A=v#x\n", "A", "v#x", true},
		{"multiline", "A=\"one\ntwo\"\n", "A", "one\ntwo", true},
		{"export", "export A=1\n", "A", "1", true},
		{"empty before comment", "A= # c\n", "A", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Parse([]byte(tc.input)).Get(tc.key)
			if got != tc.want || ok != tc.ok {
				t.Errorf("Get(%q) = %q, %v; want %q, %v", tc.key, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestSet(t *testing.T) {
	cases := []struct {
		name  string
		input string
		key   string
		value string
		want  string
	}{
		{"update in place", "A=1\nB=2\n", "A", "9", "A=9\nB=2\n"},
		{"keeps inline comment", "A=1 # keep me\n", "A", "2", "A=2 # keep me\n"},
		{"keeps spacing", "A = 1\n", "A", "2", "A = 2\n"},
		{"keeps export", "export A=1\n", "A", "2", "export A=2\n"},
		{"keeps double quotes", `A="old"` + "\n", "A", "new", `A="new"` + "\n"},
		{"keeps single quotes", "A='old'\n", "A", "new", "A='new'\n"},
		{"quotes when needed", "A=plain\n", "A", "has space", "A=\"has space\"\n"},
		{"escapes on quote", "A=x\n", "A", "say \"hi\"\n", `A="say \"hi\"\n"` + "\n"},
		{"single to double on apostrophe", "A='x'\n", "A", "it's", `A="it's"` + "\n"},
		{"append new key", "A=1\n", "B", "2", "A=1\nB=2\n"},
		{"append adds newline first", "A=1", "B", "2", "A=1\nB=2\n"},
		{"append respects crlf", "A=1\r\n", "B", "2", "A=1\r\nB=2\r\n"},
		{"append to empty", "", "A", "1", "A=1\n"},
		{"updates last duplicate", "A=1\nA=2\n", "A", "3", "A=1\nA=3\n"},
		{"untouched lines stay verbatim", "# c\n\nB='q'  # x\nA=1\n", "A", "2", "# c\n\nB='q'  # x\nA=2\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := Parse([]byte(tc.input))
			f.Set(tc.key, tc.value)
			if got := string(f.Render()); got != tc.want {
				t.Errorf("Set(%q, %q)\n in: %q\ngot: %q\nwant: %q", tc.key, tc.value, tc.input, got, tc.want)
			}
			if got, _ := f.Get(tc.key); got != tc.value {
				t.Errorf("Get after Set = %q, want %q", got, tc.value)
			}
		})
	}
}

func TestUnset(t *testing.T) {
	f := Parse([]byte("# keep\nA=1\nB=2\nA=3\n"))
	if !f.Unset("A") {
		t.Fatal("Unset(A) = false, want true")
	}
	if got, want := string(f.Render()), "# keep\nB=2\n"; got != want {
		t.Errorf("after Unset: %q, want %q", got, want)
	}
	if f.Unset("A") {
		t.Error("second Unset(A) = true, want false")
	}
}

func TestKeys(t *testing.T) {
	f := Parse([]byte("B=1\nA=2\nB=3\n# c\n"))
	if got, want := f.Keys(), []string{"B", "A"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
}

func TestMalformedLines(t *testing.T) {
	f := Parse([]byte("A=1\ngarbage\nB=2\n=nokey\n"))
	if got, want := f.MalformedLines(), []int{2, 4}; !reflect.DeepEqual(got, want) {
		t.Errorf("MalformedLines() = %v, want %v", got, want)
	}
}

func TestMultilineConsumesCorrectly(t *testing.T) {
	f := Parse([]byte("A=\"one\ntwo\"\nB=2\n"))
	if len(f.Lines) != 2 {
		t.Fatalf("got %d logical lines, want 2", len(f.Lines))
	}
	if f.Lines[0].Kind != Entry || f.Lines[1].Kind != Entry {
		t.Errorf("kinds = %v, %v; want Entry, Entry", f.Lines[0].Kind, f.Lines[1].Kind)
	}
	if f.Lines[1].StartLine != 3 {
		t.Errorf("B starts at line %d, want 3", f.Lines[1].StartLine)
	}
}
