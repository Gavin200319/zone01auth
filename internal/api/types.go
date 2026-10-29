package api

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/gofiber/fiber/v2"

	"z01auth/internal/geo"
	"z01auth/internal/proximity"
	"z01auth/internal/store"
)

type locationIn struct {
	Lat       *float64 `json:"lat"`
	Lng       *float64 `json:"lng"`
	AccuracyM float64  `json:"accuracy_m"`
	BSSID     string   `json:"bssid"`
	SSID      string   `json:"ssid"`
	IsMock    bool     `json:"is_mock"`
}

type deviceIn struct {
	DeviceID           string `json:"device_id"`
	Platform           string `json:"platform"`
	Name               string `json:"name"`
	BiometricPublicKey string `json:"biometric_public_key"`
}

type userView struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Initials string `json:"initials"`
	Picture  string `json:"picture"`
}

type settingsView struct {
	Theme             string `json:"theme"`
	BackgroundPolling bool   `json:"background_polling"`
	ProximityLocked   bool   `json:"proximity_locked"`
}

type deviceView struct {
	ID                string `json:"id"`
	DeviceID          string `json:"device_id"`
	Platform          string `json:"platform"`
	Name              string `json:"name"`
	BiometricEnrolled bool   `json:"biometric_enrolled"`
	CreatedAt         string `json:"created_at"`
	LastSeenAt        string `json:"last_seen_at"`
}

type authResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresIn    int          `json:"expires_in"`
	User         userView     `json:"user"`
	Settings     settingsView `json:"settings"`
	Device       deviceView   `json:"device"`
}

func parseBody(c *fiber.Ctx, v any) error {
	if err := json.Unmarshal(c.Body(), v); err != nil {
		return badRequest("request body must be valid JSON")
	}
	return nil
}

func initials(name, email string) string {
	src := strings.TrimSpace(name)
	if src == "" {
		src = email
		if i := strings.IndexByte(src, '@'); i > 0 {
			src = src[:i]
		}
	}
	parts := strings.FieldsFunc(src, func(r rune) bool {
		return r == ' ' || r == '.' || r == '_' || r == '-'
	})
	if len(parts) == 0 {
		return "?"
	}
	var out []rune
	for _, p := range parts {
		out = append(out, unicode.ToUpper([]rune(p)[0]))
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 1 {
		if r := []rune(parts[0]); len(r) > 1 {
			out = append(out, unicode.ToUpper(r[1]))
		}
	}
	return string(out)
}

func toUserView(u store.User) userView {
	return userView{ID: u.ID, Email: u.Email, Name: u.Name, Initials: initials(u.Name, u.Email), Picture: u.Picture}
}

func toSettingsView(s store.Settings) settingsView {
	return settingsView{Theme: s.Theme, BackgroundPolling: s.BackgroundPolling, ProximityLocked: true}
}

func toDeviceView(d store.Device) deviceView {
	return deviceView{
		ID:                d.ID,
		DeviceID:          d.DeviceID,
		Platform:          d.Platform,
		Name:              d.Label,
		BiometricEnrolled: d.BiometricKey != "",
		CreatedAt:         d.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		LastSeenAt:        d.LastSeenAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func readLocation(l locationIn) (proximity.Reading, error) {
	var r proximity.Reading
	if l.IsMock {
		return r, newErr(403, "MOCK_LOCATION", "mock locations are not allowed")
	}
	if (l.Lat == nil) != (l.Lng == nil) {
		return r, badRequest("lat and lng must be sent together")
	}
	if l.Lat != nil && !geo.ValidCoords(*l.Lat, *l.Lng) {
		return r, badRequest("lat/lng are out of range")
	}
	if l.AccuracyM < 0 {
		return r, badRequest("accuracy_m must not be negative")
	}
	if len(l.BSSID) > 32 || len(l.SSID) > 64 {
		return r, badRequest("bssid or ssid is too long")
	}
	return proximity.Reading{Lat: l.Lat, Lng: l.Lng, AccuracyM: l.AccuracyM, BSSID: l.BSSID, SSID: l.SSID}, nil
}

func validateDevice(d deviceIn) error {
	if !store.ValidDeviceID(d.DeviceID) {
		return badRequest("device.device_id must be 8-128 characters of letters, digits, '-' or '_'")
	}
	if d.Platform != "android" && d.Platform != "ios" {
		return badRequest("device.platform must be android or ios")
	}
	if len(d.Name) > 100 {
		return badRequest("device.name is too long")
	}
	return nil
}
