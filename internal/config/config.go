package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"z01auth/internal/geo"
)

type Premises struct {
	Name      string
	SSID      string
	Lat       float64
	Lng       float64
	GeofenceM float64
	FarM      float64
	BSSIDs    []string
}

type Config struct {
	Env                    string
	Port                   string
	DatabaseURL            string
	FirebaseCredentialsFile string
	FirebaseCredentialsJSON string
	JWTSecret              []byte
	AccessTTL              time.Duration
	RefreshTTL             time.Duration
	GoogleClientIDs        []string
	AllowedEmailDomains    []string
	DevAuth                bool
	Location               *time.Location
	CORSOrigins            string
	TrustProxy             bool
	MaxGPSAccuracyM        float64
	RequireBiometricTimeIn bool
	HeartbeatSeconds       int
	Premises               Premises
}

type loader struct{ errs []string }

func (l *loader) fail(format string, a ...any) {
	l.errs = append(l.errs, fmt.Sprintf(format, a...))
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func list(key string) []string {
	var out []string
	for _, p := range strings.Split(os.Getenv(key), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (l *loader) float(key string, def float64) float64 {
	v := env(key, "")
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		l.fail("%s must be a number", key)
		return def
	}
	return f
}

func (l *loader) requiredFloat(key string) float64 {
	if env(key, "") == "" {
		l.fail("%s is required", key)
		return 0
	}
	return l.float(key, 0)
}

func (l *loader) integer(key string, def int) int {
	v := env(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.fail("%s must be an integer", key)
		return def
	}
	return n
}

func (l *loader) boolean(key string, def bool) bool {
	v := env(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.fail("%s must be true or false", key)
		return def
	}
	return b
}

func (l *loader) duration(key string, def time.Duration) time.Duration {
	v := env(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		l.fail("%s must be a positive duration like 15m or 720h", key)
		return def
	}
	return d
}

func Load() (*Config, error) {
	l := &loader{}
	c := &Config{
		Env:                    env("APP_ENV", "development"),
		Port:                   env("PORT", "8080"),
		DatabaseURL:            env("DATABASE_URL", ""),
		FirebaseCredentialsFile: env("FIREBASE_CREDENTIALS_FILE", env("GOOGLE_APPLICATION_CREDENTIALS", "")),
		FirebaseCredentialsJSON: env("FIREBASE_CREDENTIALS_JSON", ""),
		AccessTTL:              l.duration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTTL:             l.duration("REFRESH_TOKEN_TTL", 720*time.Hour),
		GoogleClientIDs:        list("GOOGLE_CLIENT_IDS"),
		DevAuth:                l.boolean("DEV_AUTH", false),
		CORSOrigins:            env("CORS_ORIGINS", ""),
		TrustProxy:             l.boolean("TRUST_PROXY", false),
		MaxGPSAccuracyM:        l.float("MAX_GPS_ACCURACY_M", 100),
		RequireBiometricTimeIn: l.boolean("REQUIRE_BIOMETRIC_TIME_IN", true),
		HeartbeatSeconds:       l.integer("HEARTBEAT_INTERVAL_SECONDS", 300),
	}
	for _, d := range list("ALLOWED_EMAIL_DOMAINS") {
		c.AllowedEmailDomains = append(c.AllowedEmailDomains, strings.ToLower(strings.TrimPrefix(d, "@")))
	}

	if !strings.HasPrefix(c.DatabaseURL, "https://") && !strings.HasPrefix(c.DatabaseURL, "http://") {
		l.fail("DATABASE_URL must be your Firebase Realtime Database URL, e.g. https://<project>-default-rtdb.firebaseio.com/")
	}
	if c.FirebaseCredentialsFile == "" && c.FirebaseCredentialsJSON == "" {
		l.fail("set FIREBASE_CREDENTIALS_FILE (service account key path) or FIREBASE_CREDENTIALS_JSON")
	}
	secret := env("JWT_SECRET", "")
	if len(secret) < 32 {
		l.fail("JWT_SECRET must be at least 32 characters")
	}
	c.JWTSecret = []byte(secret)

	if c.DevAuth && c.Env == "production" {
		l.fail("DEV_AUTH cannot be enabled when APP_ENV=production")
	}
	if !c.DevAuth && len(c.GoogleClientIDs) == 0 {
		l.fail("GOOGLE_CLIENT_IDS is required unless DEV_AUTH=true")
	}

	loc, err := time.LoadLocation(env("TIMEZONE", "Africa/Nairobi"))
	if err != nil {
		l.fail("TIMEZONE is invalid: %v", err)
	}
	c.Location = loc

	p := Premises{
		Name:      env("PREMISES_NAME", "Zone01 Campus"),
		SSID:      env("PREMISES_SSID", "Z01_Campus_5G"),
		Lat:       l.requiredFloat("PREMISES_LAT"),
		Lng:       l.requiredFloat("PREMISES_LNG"),
		GeofenceM: l.float("GEOFENCE_RADIUS_M", 50),
		FarM:      l.float("FAR_RADIUS_M", 150),
	}
	if !geo.ValidCoords(p.Lat, p.Lng) {
		l.fail("PREMISES_LAT/PREMISES_LNG are out of range")
	}
	if p.GeofenceM <= 0 || p.FarM < p.GeofenceM {
		l.fail("GEOFENCE_RADIUS_M must be > 0 and FAR_RADIUS_M must be >= GEOFENCE_RADIUS_M")
	}
	for _, b := range list("PREMISES_BSSIDS") {
		n := geo.NormalizeBSSID(b)
		if n == "" {
			l.fail("PREMISES_BSSIDS contains an invalid BSSID: %s", b)
			continue
		}
		p.BSSIDs = append(p.BSSIDs, n)
	}
	c.Premises = p

	if len(l.errs) > 0 {
		return nil, fmt.Errorf("invalid configuration: %s", strings.Join(l.errs, "; "))
	}
	return c, nil
}
