package proximity

import (
	"testing"

	"z01auth/internal/store"
)

func campus() []store.Premises {
	return []store.Premises{{
		ID:        "p1",
		Name:      "Campus",
		Lat:       -0.1,
		Lng:       34.75,
		GeofenceM: 50,
		FarM:      150,
		BSSIDs:    map[string]struct{}{"00:1a:2b:3c:4d:5e": {}},
	}}
}

func ptr(f float64) *float64 { return &f }

func TestInsideGeofence(t *testing.T) {
	r := Evaluate(campus(), Reading{Lat: ptr(-0.1), Lng: ptr(34.75), AccuracyM: 10}, 100)
	if !r.Unlocked || r.Zone != ZoneOn || r.GPSStatus != "inside" {
		t.Fatalf("expected on premises, got %+v", r)
	}
}

func TestWifiOnly(t *testing.T) {
	r := Evaluate(campus(), Reading{BSSID: "00:1A:2B:3C:4D:5E"}, 100)
	if !r.Unlocked || !r.WifiOK || r.WifiStatus != "verified" {
		t.Fatalf("expected wifi unlock, got %+v", r)
	}
}

func TestApproachingStaysLocked(t *testing.T) {
	// roughly 100m north of the centre
	r := Evaluate(campus(), Reading{Lat: ptr(-0.1 + 0.0009), Lng: ptr(34.75), AccuracyM: 5}, 100)
	if r.Unlocked || r.Zone != ZoneApproaching {
		t.Fatalf("expected approaching and locked, got %+v", r)
	}
}

func TestOffPremises(t *testing.T) {
	r := Evaluate(campus(), Reading{Lat: ptr(-0.2), Lng: ptr(34.8), AccuracyM: 5, BSSID: "aa:bb:cc:dd:ee:ff"}, 100)
	if r.Unlocked || r.Zone != ZoneOff || r.WifiStatus != "unverified" {
		t.Fatalf("expected off premises, got %+v", r)
	}
}

func TestPoorAccuracyIgnored(t *testing.T) {
	r := Evaluate(campus(), Reading{Lat: ptr(-0.1), Lng: ptr(34.75), AccuracyM: 500}, 100)
	if r.Unlocked || r.GPSStatus != "unknown" {
		t.Fatalf("expected inaccurate gps to be ignored, got %+v", r)
	}
}

func TestPlaceholderBSSIDIgnored(t *testing.T) {
	r := Evaluate(campus(), Reading{BSSID: "02:00:00:00:00:00"}, 100)
	if r.WifiStatus != "none" {
		t.Fatalf("expected placeholder bssid ignored, got %+v", r)
	}
}
