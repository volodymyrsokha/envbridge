package envfile

import "testing"

func FuzzRoundTrip(f *testing.F) {
	for _, tc := range roundTripCases {
		f.Add([]byte(tc.input))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		got := Parse(data).Render()
		if string(got) != string(data) {
			t.Errorf("round trip changed bytes\n in: %q\nout: %q", data, got)
		}
	})
}
