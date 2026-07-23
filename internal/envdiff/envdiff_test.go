package envdiff

import (
	"reflect"
	"testing"

	"github.com/volodymyrsokha/envbridge/internal/envfile"
)

func TestDiff(t *testing.T) {
	cases := []struct {
		name string
		from string
		to   string
		want []Change
	}{
		{"identical", "A=1\nB=2\n", "A=1\nB=2\n", nil},
		{"formatting only", "A=1 # c\n\nB=2\n", "B=2\nA=1\n", nil},
		{"added", "A=1\n", "A=1\nB=2\n", []Change{{Kind: Added, Key: "B", New: "2"}}},
		{"removed", "A=1\nB=2\n", "A=1\n", []Change{{Kind: Removed, Key: "B", Old: "2"}}},
		{"changed", "A=1\n", "A=2\n", []Change{{Kind: Changed, Key: "A", Old: "1", New: "2"}}},
		{"quote style change is invisible", "A=hello\n", `A="hello"` + "\n", nil},
		{
			"mixed keeps order",
			"A=1\nB=2\nC=3\n",
			"A=9\nC=3\nD=4\n",
			[]Change{
				{Kind: Changed, Key: "A", Old: "1", New: "9"},
				{Kind: Removed, Key: "B", Old: "2"},
				{Kind: Added, Key: "D", New: "4"},
			},
		},
		{"duplicate last wins", "A=1\nA=2\n", "A=2\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Diff(envfile.Parse([]byte(tc.from)), envfile.Parse([]byte(tc.to)))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Diff\nfrom: %q\n  to: %q\n got: %+v\nwant: %+v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestMask(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "(empty)"},
		{"x", "••••••••"},
		{"shortvalue", "••••••••"},
		{"sk_live_abcdef123456", "••••3456"},
		{"постгрес-секрет", "••••крет"},
	}
	for _, tc := range cases {
		if got := Mask(tc.in); got != tc.want {
			t.Errorf("Mask(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
