package store

import (
	"context"
	"log"
	"time"
)

// PurgeExpiredChallenges removes challenges whose expires_at timestamp is older than cutoff.
func (s *Store) PurgeExpiredChallenges(ctx context.Context, cutoff time.Time) (int, error) {
	nodes, err := s.ref("challenges").OrderByChild("expires_at").EndAt(ms(cutoff)).GetOrdered(ctx)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, n := range nodes {
		if err := s.ref("challenges/" + n.Key()).Delete(ctx); err == nil {
			deleted++
		}
	}
	return deleted, nil
}

// PurgeOldEvents removes audit events for a user older than the cutoff timestamp.
func (s *Store) PurgeOldEvents(ctx context.Context, userID string, cutoff time.Time) (int, error) {
	nodes, err := s.ref("events/" + userID).OrderByKey().EndAt(eventKey(cutoff)).GetOrdered(ctx)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, n := range nodes {
		if err := s.ref("events/" + userID + "/" + n.Key()).Delete(ctx); err == nil {
			deleted++
		}
	}
	return deleted, nil
}

// StartCleanupWorker runs a background routine to purge stale challenges periodically.
func (s *Store) StartCleanupWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				// Delete challenges expired over 1 hour ago
				if count, err := s.PurgeExpiredChallenges(ctx, now.Add(-1*time.Hour)); err != nil {
					log.Printf("cleanup: purge challenges error: %v", err)
				} else if count > 0 {
					log.Printf("cleanup: purged %d expired challenges", count)
				}
			}
		}
	}()
}
