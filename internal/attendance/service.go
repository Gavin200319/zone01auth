package attendance

import (
	"context"
	"errors"
	"math"
	"time"

	"z01auth/internal/proximity"
	"z01auth/internal/store"
)

const (
	StateCheckedIn = "checked_in"
	StateNear      = "near_premises"
	StateOff       = "off_premises"
)

var ErrInvalidMonth = errors.New("month must be formatted as YYYY-MM")

type Service struct {
	Store        *store.Store
	Premises     []store.Premises
	Loc          *time.Location
	MaxAccuracyM float64
	RequireBio   bool
	Now          func() time.Time
}

type ProximityView struct {
	Zone               string   `json:"zone"`
	DistanceM          *float64 `json:"distance_m"`
	WifiStatus         string   `json:"wifi_status"`
	GPSStatus          string   `json:"gps_status"`
	BiometricsUnlocked bool     `json:"biometrics_unlocked"`
	PremisesName       string   `json:"premises_name"`
	SSID               string   `json:"ssid"`
}

type RadarView struct {
	State string `json:"state"`
	ProximityView
	BiometricVerified bool `json:"biometric_verified"`
	CheckInRequired   bool `json:"check_in_required"`
}

type EventView struct {
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Detail     string    `json:"detail"`
	BSSID      string    `json:"bssid,omitempty"`
	RecordedAt time.Time `json:"recorded_at"`
}

type TodayView struct {
	Date          string      `json:"date"`
	Timezone      string      `json:"timezone"`
	CheckedIn     bool        `json:"checked_in"`
	FirstSeenAt   *time.Time  `json:"first_seen_at"`
	LastSeenAt    *time.Time  `json:"last_seen_at"`
	ActiveSeconds int64       `json:"active_seconds"`
	Verified      bool        `json:"verified"`
	TimeInLocked  bool        `json:"time_in_locked"`
	RecentEvents  []EventView `json:"recent_events"`
}

type HeartbeatResult struct {
	ServerTime time.Time `json:"server_time"`
	Radar      RadarView `json:"radar"`
	Today      TodayView `json:"today"`
}

type HeartbeatInput struct {
	UserID      string
	DeviceID    string
	Reading     proximity.Reading
	Manual      bool
	BiometricOK bool
}

type DayView struct {
	Date            string    `json:"date"`
	FirstSeenAt     time.Time `json:"first_seen_at"`
	LastSeenAt      time.Time `json:"last_seen_at"`
	DurationSeconds int64     `json:"duration_seconds"`
	IsToday         bool      `json:"is_today"`
	Status          string    `json:"status"`
	Verified        bool      `json:"verified"`
}

type SummaryView struct {
	PresentDays     int     `json:"present_days"`
	AvgHours        float64 `json:"avg_hours"`
	VerifiedPercent int     `json:"verified_percent"`
}

type HistoryView struct {
	Month   string      `json:"month"`
	Summary SummaryView `json:"summary"`
	Days    []DayView   `json:"days"`
}

func (s *Service) dayOf(t time.Time) time.Time {
	y, m, d := t.In(s.Loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func (s *Service) local(t time.Time) time.Time { return t.In(s.Loc) }

func (s *Service) localPtr(t time.Time) *time.Time {
	l := s.local(t)
	return &l
}

func (s *Service) Evaluate(r proximity.Reading) proximity.Result {
	return proximity.Evaluate(s.Premises, r, s.MaxAccuracyM)
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func (s *Service) proximityView(p proximity.Result) ProximityView {
	v := ProximityView{
		Zone:               p.Zone,
		WifiStatus:         p.WifiStatus,
		GPSStatus:          p.GPSStatus,
		BiometricsUnlocked: p.Unlocked,
	}
	if p.DistanceM != nil {
		d := math.Round(*p.DistanceM)
		v.DistanceM = &d
	}
	if p.Premises != nil {
		v.PremisesName = p.Premises.Name
		v.SSID = p.Premises.SSID
	}
	return v
}

func (s *Service) Check(r proximity.Reading) ProximityView {
	return s.proximityView(s.Evaluate(r))
}

func (s *Service) radar(p proximity.Result, day *store.Day) RadarView {
	state := StateOff
	switch {
	case p.Unlocked && day != nil:
		state = StateCheckedIn
	case p.Unlocked:
		state = StateNear
	}
	return RadarView{
		State:             state,
		ProximityView:     s.proximityView(p),
		BiometricVerified: day != nil && day.Verified,
		CheckInRequired:   p.Unlocked && day == nil,
	}
}

func eventView(e store.Event, loc *time.Location) EventView {
	v := EventView{Kind: e.Kind, BSSID: e.BSSID, RecordedAt: e.RecordedAt.In(loc)}
	switch e.Kind {
	case "first_check_in":
		v.Title = "First Check-In (Time In)"
		switch {
		case e.WifiOK && e.GPSOK:
			v.Detail = "Door Wi-Fi + GPS Handshake"
		case e.WifiOK:
			v.Detail = "Door Wi-Fi Handshake"
		default:
			v.Detail = "GPS Geofence Handshake"
		}
		if e.BiometricOK {
			v.Detail += " + Biometrics"
		}
	case "heartbeat_manual":
		v.Title = "Biometric Sync (Manual)"
		v.Detail = heartbeatDetail(e)
	default:
		v.Title = "Wi-Fi Heartbeat (Auto)"
		v.Detail = heartbeatDetail(e)
	}
	return v
}

func heartbeatDetail(e store.Event) string {
	if e.BSSID != "" {
		return "BSSID: " + e.BSSID
	}
	return "GPS Geofence"
}

func (s *Service) today(ctx context.Context, userID string, day *store.Day, now time.Time) (TodayView, error) {
	v := TodayView{
		Date:         s.dayOf(now).Format("2006-01-02"),
		Timezone:     s.Loc.String(),
		TimeInLocked: true,
		RecentEvents: []EventView{},
	}
	if day != nil {
		v.CheckedIn = true
		v.FirstSeenAt = s.localPtr(day.FirstSeen)
		v.LastSeenAt = s.localPtr(day.LastSeen)
		v.ActiveSeconds = int64(day.LastSeen.Sub(day.FirstSeen).Seconds())
		v.Verified = day.Verified
	}
	y, m, d := now.In(s.Loc).Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, s.Loc)
	events, err := s.Store.RecentEvents(ctx, userID, start, 5)
	if err != nil {
		return v, err
	}
	for _, e := range events {
		v.RecentEvents = append(v.RecentEvents, eventView(e, s.Loc))
	}
	return v, nil
}

func (s *Service) Today(ctx context.Context, userID string) (TodayView, error) {
	now := s.Now()
	day, err := s.Store.GetDay(ctx, userID, s.dayOf(now))
	if err != nil {
		return TodayView{}, err
	}
	return s.today(ctx, userID, day, now)
}

func (s *Service) Heartbeat(ctx context.Context, in HeartbeatInput) (*HeartbeatResult, error) {
	now := s.Now()
	dayKey := s.dayOf(now)
	p := s.Evaluate(in.Reading)

	var day *store.Day
	var err error

	if !p.Unlocked {
		day, err = s.Store.GetDay(ctx, in.UserID, dayKey)
		if err != nil {
			return nil, err
		}
	} else {
		day, err = s.Store.TouchDay(ctx, in.UserID, dayKey, now, in.BiometricOK)
		if err != nil {
			return nil, err
		}
		inserted := false
		if day == nil && (in.BiometricOK || !s.RequireBio) {
			day, inserted, err = s.Store.CreateDay(ctx, in.UserID, p.Premises.ID, dayKey, now, in.BiometricOK)
			if err != nil {
				return nil, err
			}
		}
		if day != nil {
			kind := "heartbeat_auto"
			switch {
			case inserted:
				kind = "first_check_in"
			case in.Manual:
				kind = "heartbeat_manual"
			}
			err = s.Store.InsertEvent(ctx, store.EventInput{
				UserID:      in.UserID,
				DeviceID:    in.DeviceID,
				PremisesID:  p.Premises.ID,
				Kind:        kind,
				BSSID:       p.BSSID,
				SSID:        in.Reading.SSID,
				Lat:         in.Reading.Lat,
				Lng:         in.Reading.Lng,
				AccuracyM:   in.Reading.AccuracyM,
				DistanceM:   p.DistanceM,
				WifiOK:      p.WifiOK,
				GPSOK:       p.GPSOK,
				BiometricOK: in.BiometricOK,
				At:          now,
			})
			if err != nil {
				return nil, err
			}
		}
	}

	today, err := s.today(ctx, in.UserID, day, now)
	if err != nil {
		return nil, err
	}
	return &HeartbeatResult{ServerTime: s.local(now), Radar: s.radar(p, day), Today: today}, nil
}

func (s *Service) History(ctx context.Context, userID, month string) (*HistoryView, error) {
	now := s.Now()
	start := s.dayOf(now)
	start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	if month != "" {
		t, err := time.Parse("2006-01", month)
		if err != nil {
			return nil, ErrInvalidMonth
		}
		start = t
	}
	days, err := s.Store.ListDays(ctx, userID, start, start.AddDate(0, 1, 0))
	if err != nil {
		return nil, err
	}

	todayKey := s.dayOf(now).Format("2006-01-02")
	h := &HistoryView{Month: start.Format("2006-01"), Days: make([]DayView, 0, len(days))}
	var totalSeconds int64
	verified := 0
	for _, d := range days {
		key := d.Day.Format("2006-01-02")
		dur := int64(d.LastSeen.Sub(d.FirstSeen).Seconds())
		totalSeconds += dur
		if d.Verified {
			verified++
		}
		status := "completed"
		if key == todayKey {
			status = "active"
		}
		h.Days = append(h.Days, DayView{
			Date:            key,
			FirstSeenAt:     s.local(d.FirstSeen),
			LastSeenAt:      s.local(d.LastSeen),
			DurationSeconds: dur,
			IsToday:         key == todayKey,
			Status:          status,
			Verified:        d.Verified,
		})
	}
	if n := len(days); n > 0 {
		h.Summary = SummaryView{
			PresentDays:     n,
			AvgHours:        round1(float64(totalSeconds) / float64(n) / 3600),
			VerifiedPercent: int(math.Round(float64(verified) * 100 / float64(n))),
		}
	}
	return h, nil
}

func (s *Service) Months(ctx context.Context, userID string) ([]string, error) {
	months, err := s.Store.ListMonths(ctx, userID)
	if err != nil {
		return nil, err
	}
	current := s.dayOf(s.Now()).Format("2006-01")
	for _, m := range months {
		if m == current {
			return months, nil
		}
	}
	return append([]string{current}, months...), nil
}
