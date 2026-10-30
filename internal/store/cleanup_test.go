package store

import (
	"context"
	"testing"
	"time"
)

func TestCleanupWorkerContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	st := &Store{}
	// Calling StartCleanupWorker and cancelling immediately should return cleanly
	st.StartCleanupWorker(ctx, 50*time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestMsCutoff(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-1 * time.Hour)
	if ms(cutoff) >= ms(now) {
		t.Fatalf("cutoff ms must be strictly less than now ms")
	}
}
