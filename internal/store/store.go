package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/db"
	"google.golang.org/api/option"

	"z01auth/internal/config"
)

var (
	ErrNotFound    = errors.New("not found")
	ErrDeviceBound = errors.New("device is bound to another account")
)

type Store struct {
	c    *db.Client
	root *db.Ref
}

func Open(ctx context.Context, cfg *config.Config) (*Store, error) {
	var opts []option.ClientOption
	switch {
	case cfg.FirebaseCredentialsJSON != "":
		opts = append(opts, option.WithCredentialsJSON([]byte(cfg.FirebaseCredentialsJSON)))
	case cfg.FirebaseCredentialsFile != "":
		opts = append(opts, option.WithCredentialsFile(cfg.FirebaseCredentialsFile))
	}
	app, err := firebase.NewApp(ctx, &firebase.Config{DatabaseURL: cfg.DatabaseURL}, opts...)
	if err != nil {
		return nil, err
	}
	client, err := app.Database(ctx)
	if err != nil {
		return nil, err
	}
	return &Store{c: client, root: client.NewRef("/")}, nil
}

func (s *Store) ref(path string) *db.Ref { return s.c.NewRef(path) }

func (s *Store) Ping(ctx context.Context) error {
	var v any
	return s.ref("meta/ping").Get(ctx, &v)
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func hashKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func ValidID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func ValidDeviceID(s string) bool {
	if len(s) < 8 || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

type User struct {
	ID        string
	GoogleSub string
	Email     string
	Name      string
	Picture   string
	CreatedAt time.Time
}

type Settings struct {
	Theme             string
	BackgroundPolling bool
}

type Device struct {
	ID           string
	UserID       string
	DeviceID     string
	Platform     string
	Label        string
	BiometricKey string
	CreatedAt    time.Time
	LastSeenAt   time.Time
}

type Session struct {
	ID       string
	UserID   string
	DeviceID string
}

type Premises struct {
	ID        string
	Name      string
	SSID      string
	Lat       float64
	Lng       float64
	GeofenceM float64
	FarM      float64
	BSSIDs    map[string]struct{}
}

func (p *Premises) HasBSSID(b string) bool {
	_, ok := p.BSSIDs[b]
	return ok
}

type Day struct {
	ID         string
	PremisesID string
	Day        time.Time
	FirstSeen  time.Time
	LastSeen   time.Time
	Verified   bool
}

type Event struct {
	Kind        string
	BSSID       string
	SSID        string
	WifiOK      bool
	GPSOK       bool
	BiometricOK bool
	RecordedAt  time.Time
}

type EventInput struct {
	UserID      string
	DeviceID    string
	PremisesID  string
	Kind        string
	BSSID       string
	SSID        string
	Lat         *float64
	Lng         *float64
	AccuracyM   float64
	DistanceM   *float64
	WifiOK      bool
	GPSOK       bool
	BiometricOK bool
	At          time.Time
}

type userRec struct {
	GoogleSub string       `json:"google_sub"`
	Email     string       `json:"email"`
	Name      string       `json:"name"`
	Picture   string       `json:"picture"`
	CreatedAt int64        `json:"created_at"`
	Settings  *settingsRec `json:"settings,omitempty"`
}

type settingsRec struct {
	Theme             string `json:"theme"`
	BackgroundPolling bool   `json:"background_polling"`
}

type deviceRec struct {
	UserID       string `json:"user_id"`
	DeviceID     string `json:"device_id"`
	Platform     string `json:"platform"`
	Label        string `json:"label"`
	BiometricKey string `json:"biometric_public_key"`
	CreatedAt    int64  `json:"created_at"`
	LastSeenAt   int64  `json:"last_seen_at"`
}

type sessionRec struct {
	UserID      string `json:"user_id"`
	DeviceID    string `json:"device_id"`
	RefreshHash string `json:"refresh_hash"`
	ExpiresAt   int64  `json:"expires_at"`
	Revoked     bool   `json:"revoked"`
	CreatedAt   int64  `json:"created_at"`
}

type challengeRec struct {
	UserID    string `json:"user_id"`
	DeviceID  string `json:"device_id"`
	Purpose   string `json:"purpose"`
	Nonce     string `json:"nonce"`
	ExpiresAt int64  `json:"expires_at"`
	Used      bool   `json:"used"`
}

type premisesRec struct {
	Name      string          `json:"name"`
	SSID      string          `json:"ssid"`
	Lat       float64         `json:"lat"`
	Lng       float64         `json:"lng"`
	GeofenceM float64         `json:"geofence_radius_m"`
	FarM      float64         `json:"far_radius_m"`
	BSSIDs    map[string]bool `json:"bssids,omitempty"`
}

type dayRec struct {
	PremisesID  string `json:"premises_id"`
	FirstSeenAt int64  `json:"first_seen_at"`
	LastSeenAt  int64  `json:"last_seen_at"`
	Verified    bool   `json:"verified"`
}

type eventRec struct {
	DeviceID    string   `json:"device_id"`
	PremisesID  string   `json:"premises_id"`
	Kind        string   `json:"kind"`
	BSSID       string   `json:"bssid"`
	SSID        string   `json:"ssid"`
	Lat         *float64 `json:"lat,omitempty"`
	Lng         *float64 `json:"lng,omitempty"`
	AccuracyM   float64  `json:"accuracy_m"`
	DistanceM   *float64 `json:"distance_m,omitempty"`
	WifiOK      bool     `json:"wifi_ok"`
	GPSOK       bool     `json:"gps_ok"`
	BiometricOK bool     `json:"biometric_ok"`
	RecordedAt  int64    `json:"recorded_at"`
}

func ms(t time.Time) int64 { return t.UnixMilli() }

func fromMS(v int64) time.Time { return time.UnixMilli(v).UTC() }

func defaultSettings() Settings { return Settings{Theme: "dark", BackgroundPolling: true} }

func (s *Store) UpsertUser(ctx context.Context, sub, email, name, picture string) (User, error) {
	idx := s.ref("google_subs/" + hashKey(sub))
	candidate := newID()
	var uid string
	err := idx.Transaction(ctx, func(n db.TransactionNode) (any, error) {
		var cur string
		if err := n.Unmarshal(&cur); err != nil {
			return nil, err
		}
		if cur != "" {
			uid = cur
			return cur, nil
		}
		uid = candidate
		return candidate, nil
	})
	if err != nil {
		return User{}, err
	}

	if uid == candidate {
		def := defaultSettings()
		rec := userRec{
			GoogleSub: sub,
			Email:     email,
			Name:      name,
			Picture:   picture,
			CreatedAt: ms(time.Now()),
			Settings:  &settingsRec{Theme: def.Theme, BackgroundPolling: def.BackgroundPolling},
		}
		if err := s.ref("users/" + uid).Set(ctx, rec); err != nil {
			return User{}, err
		}
	} else {
		upd := map[string]any{"email": email}
		if name != "" {
			upd["name"] = name
		}
		if picture != "" {
			upd["picture"] = picture
		}
		if err := s.ref("users/" + uid).Update(ctx, upd); err != nil {
			return User{}, err
		}
	}
	return s.GetUser(ctx, uid)
}

func (s *Store) GetUser(ctx context.Context, id string) (User, error) {
	if !ValidID(id) {
		return User{}, ErrNotFound
	}
	var rec *userRec
	if err := s.ref("users/"+id).Get(ctx, &rec); err != nil {
		return User{}, err
	}
	if rec == nil {
		return User{}, ErrNotFound
	}
	return User{
		ID:        id,
		GoogleSub: rec.GoogleSub,
		Email:     rec.Email,
		Name:      rec.Name,
		Picture:   rec.Picture,
		CreatedAt: fromMS(rec.CreatedAt),
	}, nil
}

func (s *Store) GetSettings(ctx context.Context, userID string) (Settings, error) {
	var rec *settingsRec
	if err := s.ref("users/"+userID+"/settings").Get(ctx, &rec); err != nil {
		return Settings{}, err
	}
	if rec == nil {
		return defaultSettings(), nil
	}
	return Settings{Theme: rec.Theme, BackgroundPolling: rec.BackgroundPolling}, nil
}

func (s *Store) UpdateSettings(ctx context.Context, userID string, theme *string, polling *bool) (Settings, error) {
	var out Settings
	err := s.ref("users/"+userID+"/settings").Transaction(ctx, func(n db.TransactionNode) (any, error) {
		var cur *settingsRec
		if err := n.Unmarshal(&cur); err != nil {
			return nil, err
		}
		d := defaultSettings()
		next := settingsRec{Theme: d.Theme, BackgroundPolling: d.BackgroundPolling}
		if cur != nil {
			next = *cur
		}
		if theme != nil {
			next.Theme = *theme
		}
		if polling != nil {
			next.BackgroundPolling = *polling
		}
		out = Settings{Theme: next.Theme, BackgroundPolling: next.BackgroundPolling}
		return next, nil
	})
	return out, err
}

func toDevice(id string, r deviceRec) Device {
	return Device{
		ID:           id,
		UserID:       r.UserID,
		DeviceID:     r.DeviceID,
		Platform:     r.Platform,
		Label:        r.Label,
		BiometricKey: r.BiometricKey,
		CreatedAt:    fromMS(r.CreatedAt),
		LastSeenAt:   fromMS(r.LastSeenAt),
	}
}

func (s *Store) BindDevice(ctx context.Context, userID, deviceID, platform, label, bioKey string) (Device, error) {
	if !ValidDeviceID(deviceID) {
		return Device{}, ErrNotFound
	}
	candidate := newID()
	var rowID string
	err := s.ref("device_index/"+deviceID).Transaction(ctx, func(n db.TransactionNode) (any, error) {
		var cur string
		if err := n.Unmarshal(&cur); err != nil {
			return nil, err
		}
		if cur != "" {
			rowID = cur
			return cur, nil
		}
		rowID = candidate
		return candidate, nil
	})
	if err != nil {
		return Device{}, err
	}

	now := ms(time.Now())
	if rowID == candidate {
		rec := deviceRec{
			UserID:       userID,
			DeviceID:     deviceID,
			Platform:     platform,
			Label:        label,
			BiometricKey: bioKey,
			CreatedAt:    now,
			LastSeenAt:   now,
		}
		err := s.root.Update(ctx, map[string]any{
			"devices/" + rowID:                  rec,
			"user_devices/" + userID + "/" + rowID: true,
		})
		if err != nil {
			return Device{}, err
		}
		return toDevice(rowID, rec), nil
	}

	var rec *deviceRec
	if err := s.ref("devices/"+rowID).Get(ctx, &rec); err != nil {
		return Device{}, err
	}
	if rec == nil || rec.UserID != userID {
		return Device{}, ErrDeviceBound
	}
	upd := map[string]any{"platform": platform, "last_seen_at": now}
	if label != "" {
		upd["label"] = label
	}
	if bioKey != "" {
		upd["biometric_public_key"] = bioKey
	}
	if err := s.ref("devices/" + rowID).Update(ctx, upd); err != nil {
		return Device{}, err
	}
	return s.GetDevice(ctx, rowID)
}

func (s *Store) GetDevice(ctx context.Context, id string) (Device, error) {
	if !ValidID(id) {
		return Device{}, ErrNotFound
	}
	var rec *deviceRec
	if err := s.ref("devices/"+id).Get(ctx, &rec); err != nil {
		return Device{}, err
	}
	if rec == nil {
		return Device{}, ErrNotFound
	}
	return toDevice(id, *rec), nil
}

func (s *Store) FindBiometricDevice(ctx context.Context, deviceID string) (Device, error) {
	if !ValidDeviceID(deviceID) {
		return Device{}, ErrNotFound
	}
	var rowID string
	if err := s.ref("device_index/"+deviceID).Get(ctx, &rowID); err != nil {
		return Device{}, err
	}
	if rowID == "" {
		return Device{}, ErrNotFound
	}
	d, err := s.GetDevice(ctx, rowID)
	if err != nil {
		return Device{}, err
	}
	if d.BiometricKey == "" {
		return Device{}, ErrNotFound
	}
	return d, nil
}

func (s *Store) SetBiometricKey(ctx context.Context, id, key string) error {
	if !ValidID(id) {
		return ErrNotFound
	}
	return s.ref("devices/"+id).Update(ctx, map[string]any{
		"biometric_public_key": key,
		"last_seen_at":         ms(time.Now()),
	})
}

func (s *Store) ListDevices(ctx context.Context, userID string) ([]Device, error) {
	var ids map[string]bool
	if err := s.ref("user_devices/"+userID).Get(ctx, &ids); err != nil {
		return nil, err
	}
	out := []Device{}
	for id := range ids {
		d, err := s.GetDevice(ctx, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) DeleteDevice(ctx context.Context, userID, id string) (bool, error) {
	if !ValidID(id) {
		return false, nil
	}
	var rec *deviceRec
	if err := s.ref("devices/"+id).Get(ctx, &rec); err != nil {
		return false, err
	}
	if rec == nil || rec.UserID != userID {
		return false, nil
	}

	upd := map[string]any{
		"devices/" + id:                  nil,
		"device_index/" + rec.DeviceID:   nil,
		"user_devices/" + userID + "/" + id: nil,
		"device_sessions/" + id:          nil,
	}
	var sids map[string]bool
	if err := s.ref("device_sessions/"+id).Get(ctx, &sids); err != nil {
		return false, err
	}
	for sid := range sids {
		var ss *sessionRec
		if err := s.ref("sessions/"+sid).Get(ctx, &ss); err != nil {
			return false, err
		}
		upd["sessions/"+sid] = nil
		if ss != nil && ss.RefreshHash != "" {
			upd["refresh_index/"+ss.RefreshHash] = nil
		}
	}
	if err := s.root.Update(ctx, upd); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) CreateSession(ctx context.Context, userID, deviceID, hash string, expires time.Time) (string, error) {
	id := newID()
	rec := sessionRec{
		UserID:      userID,
		DeviceID:    deviceID,
		RefreshHash: hash,
		ExpiresAt:   ms(expires),
		CreatedAt:   ms(time.Now()),
	}
	err := s.root.Update(ctx, map[string]any{
		"sessions/" + id:                        rec,
		"refresh_index/" + hash:                 id,
		"device_sessions/" + deviceID + "/" + id: true,
	})
	return id, err
}

func (s *Store) SessionActive(ctx context.Context, id string) (bool, error) {
	if !ValidID(id) {
		return false, nil
	}
	var rec *sessionRec
	if err := s.ref("sessions/"+id).Get(ctx, &rec); err != nil {
		return false, err
	}
	return rec != nil && !rec.Revoked && rec.ExpiresAt > ms(time.Now()), nil
}

func (s *Store) RotateSession(ctx context.Context, oldHash, newHash string, expires time.Time) (Session, error) {
	var sid string
	if err := s.ref("refresh_index/"+oldHash).Get(ctx, &sid); err != nil {
		return Session{}, err
	}
	if !ValidID(sid) {
		return Session{}, ErrNotFound
	}

	now := ms(time.Now())
	ok := false
	var out Session
	err := s.ref("sessions/"+sid).Transaction(ctx, func(n db.TransactionNode) (any, error) {
		ok = false
		var cur *sessionRec
		if err := n.Unmarshal(&cur); err != nil {
			return nil, err
		}
		if cur == nil {
			return nil, nil
		}
		if cur.RefreshHash != oldHash || cur.Revoked || cur.ExpiresAt <= now {
			return cur, nil
		}
		next := *cur
		next.RefreshHash = newHash
		next.ExpiresAt = ms(expires)
		ok = true
		out = Session{ID: sid, UserID: cur.UserID, DeviceID: cur.DeviceID}
		return next, nil
	})
	if err != nil {
		return Session{}, err
	}
	if !ok {
		return Session{}, ErrNotFound
	}
	err = s.root.Update(ctx, map[string]any{
		"refresh_index/" + newHash: sid,
		"refresh_index/" + oldHash: nil,
	})
	return out, err
}

func (s *Store) RevokeSession(ctx context.Context, id string) error {
	if !ValidID(id) {
		return nil
	}
	var rec *sessionRec
	if err := s.ref("sessions/"+id).Get(ctx, &rec); err != nil {
		return err
	}
	if rec == nil {
		return nil
	}
	upd := map[string]any{"sessions/" + id + "/revoked": true}
	if rec.RefreshHash != "" {
		upd["refresh_index/"+rec.RefreshHash] = nil
	}
	return s.root.Update(ctx, upd)
}

func (s *Store) CreateChallenge(ctx context.Context, userID, deviceID, purpose, nonce string, expires time.Time) (string, error) {
	id := newID()
	err := s.ref("challenges/"+id).Set(ctx, challengeRec{
		UserID:    userID,
		DeviceID:  deviceID,
		Purpose:   purpose,
		Nonce:     nonce,
		ExpiresAt: ms(expires),
	})
	return id, err
}

func (s *Store) ConsumeChallenge(ctx context.Context, id, deviceID, purpose string) (string, error) {
	if !ValidID(id) {
		return "", ErrNotFound
	}
	now := ms(time.Now())
	nonce := ""
	err := s.ref("challenges/"+id).Transaction(ctx, func(n db.TransactionNode) (any, error) {
		nonce = ""
		var cur *challengeRec
		if err := n.Unmarshal(&cur); err != nil {
			return nil, err
		}
		if cur == nil {
			return nil, nil
		}
		if cur.DeviceID != deviceID || cur.Purpose != purpose || cur.Used || cur.ExpiresAt <= now {
			return cur, nil
		}
		next := *cur
		next.Used = true
		nonce = cur.Nonce
		return next, nil
	})
	if err != nil {
		return "", err
	}
	if nonce == "" {
		return "", ErrNotFound
	}
	return nonce, nil
}

func premisesID(name string) string { return hashKey("premises:" + name)[:32] }

func (s *Store) SeedPremises(ctx context.Context, p config.Premises) error {
	rec := premisesRec{
		Name:      p.Name,
		SSID:      p.SSID,
		Lat:       p.Lat,
		Lng:       p.Lng,
		GeofenceM: p.GeofenceM,
		FarM:      p.FarM,
		BSSIDs:    map[string]bool{},
	}
	for _, b := range p.BSSIDs {
		rec.BSSIDs[b] = true
	}
	return s.ref("premises/" + premisesID(p.Name)).Set(ctx, rec)
}

func (s *Store) LoadPremises(ctx context.Context) ([]Premises, error) {
	var recs map[string]premisesRec
	if err := s.ref("premises").Get(ctx, &recs); err != nil {
		return nil, err
	}
	out := make([]Premises, 0, len(recs))
	for id, r := range recs {
		p := Premises{
			ID:        id,
			Name:      r.Name,
			SSID:      r.SSID,
			Lat:       r.Lat,
			Lng:       r.Lng,
			GeofenceM: r.GeofenceM,
			FarM:      r.FarM,
			BSSIDs:    map[string]struct{}{},
		}
		for b := range r.BSSIDs {
			p.BSSIDs[b] = struct{}{}
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

const dateLayout = "2006-01-02"

func toDay(key string, r dayRec) *Day {
	d, _ := time.Parse(dateLayout, key)
	return &Day{
		ID:         key,
		PremisesID: r.PremisesID,
		Day:        d,
		FirstSeen:  fromMS(r.FirstSeenAt),
		LastSeen:   fromMS(r.LastSeenAt),
		Verified:   r.Verified,
	}
}

func (s *Store) dayRef(userID string, day time.Time) (*db.Ref, string) {
	key := day.Format(dateLayout)
	return s.ref("attendance/" + userID + "/" + key), key
}

func (s *Store) GetDay(ctx context.Context, userID string, day time.Time) (*Day, error) {
	ref, key := s.dayRef(userID, day)
	var rec *dayRec
	if err := ref.Get(ctx, &rec); err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	return toDay(key, *rec), nil
}

func (s *Store) TouchDay(ctx context.Context, userID string, day, at time.Time, verified bool) (*Day, error) {
	ref, key := s.dayRef(userID, day)
	var out *dayRec
	err := ref.Transaction(ctx, func(n db.TransactionNode) (any, error) {
		out = nil
		var cur *dayRec
		if err := n.Unmarshal(&cur); err != nil {
			return nil, err
		}
		if cur == nil {
			return nil, nil
		}
		next := *cur
		if t := ms(at); t > next.LastSeenAt {
			next.LastSeenAt = t
		}
		next.Verified = next.Verified || verified
		out = &next
		return next, nil
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, nil
	}
	return toDay(key, *out), nil
}

func (s *Store) CreateDay(ctx context.Context, userID, premisesID string, day, at time.Time, verified bool) (*Day, bool, error) {
	ref, key := s.dayRef(userID, day)
	var out dayRec
	inserted := false
	err := ref.Transaction(ctx, func(n db.TransactionNode) (any, error) {
		inserted = false
		var cur *dayRec
		if err := n.Unmarshal(&cur); err != nil {
			return nil, err
		}
		if cur == nil {
			inserted = true
			out = dayRec{PremisesID: premisesID, FirstSeenAt: ms(at), LastSeenAt: ms(at), Verified: verified}
			return out, nil
		}
		next := *cur
		if t := ms(at); t > next.LastSeenAt {
			next.LastSeenAt = t
		}
		next.Verified = next.Verified || verified
		out = next
		return next, nil
	})
	if err != nil {
		return nil, false, err
	}
	if inserted {
		month := day.Format("2006-01")
		if err := s.ref("attendance_months/" + userID + "/" + month).Set(ctx, true); err != nil {
			return nil, false, err
		}
	}
	return toDay(key, out), inserted, nil
}

func (s *Store) ListDays(ctx context.Context, userID string, from, to time.Time) ([]Day, error) {
	nodes, err := s.ref("attendance/" + userID).OrderByKey().
		StartAt(from.Format(dateLayout)).
		EndAt(to.AddDate(0, 0, -1).Format(dateLayout)).
		GetOrdered(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Day, 0, len(nodes))
	for i := len(nodes) - 1; i >= 0; i-- {
		var rec dayRec
		if err := nodes[i].Unmarshal(&rec); err != nil {
			return nil, err
		}
		out = append(out, *toDay(nodes[i].Key(), rec))
	}
	return out, nil
}

func (s *Store) ListMonths(ctx context.Context, userID string) ([]string, error) {
	var m map[string]bool
	if err := s.ref("attendance_months/"+userID).Get(ctx, &m); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out, nil
}

func eventKey(t time.Time) string {
	return fmt.Sprintf("%013d-%s", ms(t), newID()[:6])
}

func (s *Store) InsertEvent(ctx context.Context, e EventInput) error {
	return s.ref("events/" + e.UserID + "/" + eventKey(e.At)).Set(ctx, eventRec{
		DeviceID:    e.DeviceID,
		PremisesID:  e.PremisesID,
		Kind:        e.Kind,
		BSSID:       e.BSSID,
		SSID:        e.SSID,
		Lat:         e.Lat,
		Lng:         e.Lng,
		AccuracyM:   e.AccuracyM,
		DistanceM:   e.DistanceM,
		WifiOK:      e.WifiOK,
		GPSOK:       e.GPSOK,
		BiometricOK: e.BiometricOK,
		RecordedAt:  ms(e.At),
	})
}

func (s *Store) RecentEvents(ctx context.Context, userID string, since time.Time, limit int) ([]Event, error) {
	nodes, err := s.ref("events/" + userID).OrderByKey().
		StartAt(fmt.Sprintf("%013d", ms(since))).
		LimitToLast(limit).
		GetOrdered(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(nodes))
	for i := len(nodes) - 1; i >= 0; i-- {
		var r eventRec
		if err := nodes[i].Unmarshal(&r); err != nil {
			return nil, err
		}
		out = append(out, Event{
			Kind:        r.Kind,
			BSSID:       r.BSSID,
			SSID:        r.SSID,
			WifiOK:      r.WifiOK,
			GPSOK:       r.GPSOK,
			BiometricOK: r.BiometricOK,
			RecordedAt:  fromMS(r.RecordedAt),
		})
	}
	return out, nil
}
