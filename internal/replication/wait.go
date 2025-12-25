package replication

import (
	"context"
	"fmt"
	"time"
)

type SubscriptionProgress struct {
	SubName      string
	ReceivedLSN  string
	LatestEndLSN string
	PID          int // non-zero means worker is running
}

func (s *Subscriber) GetProgress(ctx context.Context, subName string) (SubscriptionProgress, error) {
	var p SubscriptionProgress
	err := s.db.QueryRow(ctx, `
		SELECT subname,
		       COALESCE(received_lsn::text, ''),
		       COALESCE(latest_end_lsn::text, ''),
		       COALESCE(pid, 0)
		FROM pg_stat_subscription
		WHERE subname = $1
	`, subName).Scan(&p.SubName, &p.ReceivedLSN, &p.LatestEndLSN, &p.PID)
	if err != nil {
		return SubscriptionProgress{}, fmt.Errorf("read pg_stat_subscription (%q): %w", subName, err)
	}
	return p, nil
}

func (s *Subscriber) WaitUntilCaughtUp(ctx context.Context, subName string, pollInterval time.Duration, strict bool) error {
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		p, err := s.GetProgress(ctx, subName)
		if err != nil {
			return err
		}

		if caughtUp(p, strict) {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("subscription %q not caught up: %w (received=%s latest_end=%s pid=%d)",
				subName, ctx.Err(), p.ReceivedLSN, p.LatestEndLSN, p.PID)
		case <-ticker.C:
		}
	}
}

func caughtUp(p SubscriptionProgress, strict bool) bool {
	if p.ReceivedLSN == "" || p.LatestEndLSN == "" {
		return false
	}
	if strict {
		if p.ReceivedLSN == "0/0" || p.LatestEndLSN == "0/0" {
			return false
		}
	}
	return p.ReceivedLSN == p.LatestEndLSN
}
