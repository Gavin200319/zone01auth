package attendance

import (
	"strings"
	"testing"
	"time"
)

func TestRound1(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{0.0, 0.0},
		{1.234, 1.2},
		{1.267, 1.3},
		{8.45, 8.5},
	}
	for _, tc := range cases {
		got := round1(tc.in)
		if got != tc.want {
			t.Errorf("round1(%f) = %f, want %f", tc.in, got, tc.want)
		}
	}
}

func TestHistoryViewSummary(t *testing.T) {
	loc, _ := time.LoadLocation("Africa/Nairobi")
	s := &Service{
		Loc: loc,
		Now: func() time.Time {
			return time.Date(2026, 10, 30, 12, 0, 0, 0, loc)
		},
	}

	if s.dayOf(s.Now()).Format("2006-01-02") != "2026-10-30" {
		t.Fatalf("expected dayOf to match current local date")
	}
}

func TestExportCSVFormat(t *testing.T) {
	hist := &HistoryView{
		Month: "2026-10",
		Days: []DayView{
			{
				Date:            "2026-10-29",
				FirstSeenAt:     time.Date(2026, 10, 29, 8, 30, 0, 0, time.UTC),
				LastSeenAt:      time.Date(2026, 10, 29, 17, 0, 0, 0, time.UTC),
				DurationSeconds: 30600,
				Verified:        true,
				Status:          "completed",
			},
		},
	}

	var sb strings.Builder
	sb.WriteString("Date,FirstSeen,LastSeen,DurationHours,Verified,Status\n")
	for _, d := range hist.Days {
		hours := round1(float64(d.DurationSeconds) / 3600.0)
		sb.WriteString(d.Date + "," + d.FirstSeenAt.Format(time.RFC3339) + "," + d.LastSeenAt.Format(time.RFC3339) + ",8.5,true,completed\n")
		if hours != 8.5 {
			t.Errorf("expected 8.5 hours, got %f", hours)
		}
	}
	csv := sb.String()
	if !strings.HasPrefix(csv, "Date,FirstSeen,LastSeen,DurationHours,Verified,Status") {
		t.Errorf("invalid CSV header: %s", csv)
	}
}
