package proximity

import (
	"z01auth/internal/geo"
	"z01auth/internal/store"
)

const (
	ZoneOn          = "on_premises"
	ZoneApproaching = "approaching"
	ZoneOff         = "off_premises"
)

type Reading struct {
	Lat       *float64
	Lng       *float64
	AccuracyM float64
	BSSID     string
	SSID      string
}

type Result struct {
	Zone       string
	Premises   *store.Premises
	DistanceM  *float64
	WifiStatus string
	GPSStatus  string
	WifiOK     bool
	GPSOK      bool
	Unlocked   bool
	BSSID      string
}

func Evaluate(list []store.Premises, r Reading, maxAccuracyM float64) Result {
	bssid := geo.NormalizeBSSID(r.BSSID)
	hasGPS := r.Lat != nil && r.Lng != nil && geo.ValidCoords(*r.Lat, *r.Lng)
	gpsUsable := hasGPS && (maxAccuracyM <= 0 || r.AccuracyM <= maxAccuracyM)

	base := Result{Zone: ZoneOff, WifiStatus: "none", GPSStatus: "unknown", BSSID: bssid}
	if bssid != "" {
		base.WifiStatus = "unverified"
	}

	var chosen *Result
	for i := range list {
		p := &list[i]
		wifi := bssid != "" && p.HasBSSID(bssid)
		gps := false
		var dist *float64
		if gpsUsable {
			d := geo.DistanceM(*r.Lat, *r.Lng, p.Lat, p.Lng)
			dist = &d
			gps = d <= p.GeofenceM
		}

		c := base
		c.Premises = p
		c.DistanceM = dist
		c.WifiOK = wifi
		c.GPSOK = gps
		c.Unlocked = wifi || gps
		if wifi {
			c.WifiStatus = "verified"
		}
		if gpsUsable {
			c.GPSStatus = "outside"
			if gps {
				c.GPSStatus = "inside"
			}
		}
		switch {
		case c.Unlocked:
			c.Zone = ZoneOn
		case dist != nil && *dist <= p.FarM:
			c.Zone = ZoneApproaching
		}

		if chosen == nil || better(c, *chosen) {
			cc := c
			chosen = &cc
		}
	}
	if chosen == nil {
		return base
	}
	return *chosen
}

func better(a, b Result) bool {
	if a.Unlocked != b.Unlocked {
		return a.Unlocked
	}
	if a.DistanceM != nil && b.DistanceM == nil {
		return true
	}
	if a.DistanceM != nil && b.DistanceM != nil {
		return *a.DistanceM < *b.DistanceM
	}
	return false
}
