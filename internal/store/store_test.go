package store

import "testing"

func TestValidID(t *testing.T) {
	ok := "0123456789abcdef0123456789abcdef"
	if !ValidID(ok) {
		t.Fatal("valid id rejected")
	}
	for _, bad := range []string{"", "short", ok + "0", "../" + ok[:29], "0123456789ABCDEF0123456789ABCDEF"} {
		if ValidID(bad) {
			t.Errorf("ValidID(%q) = true", bad)
		}
	}
}

func TestValidDeviceID(t *testing.T) {
	if !ValidDeviceID("abc-123_XYZ") {
		t.Fatal("valid device id rejected")
	}
	for _, bad := range []string{"short", "has/slash123", "has.dot12345", "has space 123", "a$b#c[d]efgh"} {
		if ValidDeviceID(bad) {
			t.Errorf("ValidDeviceID(%q) = true", bad)
		}
	}
}
