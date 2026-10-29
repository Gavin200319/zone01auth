package geo

import (
	"math"
	"strings"
)

const earthRadiusM = 6371008.8

func DistanceM(lat1, lng1, lat2, lng2 float64) float64 {
	const rad = math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusM * math.Asin(math.Min(1, math.Sqrt(a)))
}

func ValidCoords(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 &&
		!math.IsNaN(lat) && !math.IsNaN(lng)
}

func NormalizeBSSID(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, "-", ":")))
	if len(s) != 17 || s == "02:00:00:00:00:00" || s == "00:00:00:00:00:00" {
		return ""
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i%3 == 2 {
			if c != ':' {
				return ""
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return ""
		}
	}
	return s
}
