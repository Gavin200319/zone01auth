package geo

import "testing"

func TestDistanceM(t *testing.T) {
	d := DistanceM(0, 0, 0, 0.001)
	if d < 110 || d > 112 {
		t.Fatalf("unexpected distance %f", d)
	}
}

func TestNormalizeBSSID(t *testing.T) {
	cases := map[string]string{
		"00:1A:2B:3C:4D:5E": "00:1a:2b:3c:4d:5e",
		"00-1a-2b-3c-4d-5e": "00:1a:2b:3c:4d:5e",
		"02:00:00:00:00:00": "",
		"nonsense":          "",
		"":                  "",
	}
	for in, want := range cases {
		if got := NormalizeBSSID(in); got != want {
			t.Errorf("NormalizeBSSID(%q) = %q, want %q", in, got, want)
		}
	}
}
