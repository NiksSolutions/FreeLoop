package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Lag is how far one of the counts the store keeps from the transcripts is behind
// them: how many sessions it has not read to the end, and by how many events.
type Lag struct {
	Name     string `json:"name"`
	Sessions int    `json:"sessions"`
	Events   int64  `json:"events"`
	// OnRequest says the count is brought up to date when a page asks for it, the Usage
	// page's, so being behind until then is no fault.
	OnRequest bool `json:"onRequest"`
}

// Behind says how far behind the transcripts each count is, for a problem report
// (GUI-046): one that stopped moving shows here, as the Usage page's counts did for
// four days without a word.
func (s *Store) Behind(ctx context.Context) ([]Lag, error) {
	recent := s.now().Add(-recentKeep).UnixMilli()
	counts := []struct {
		lag   Lag
		query string
	}{
		{Lag{Name: "search index"}, `SELECT count(*), sum(last_seq - indexed_seq) FROM sessions WHERE indexed_seq < last_seq`},
		{Lag{Name: "recent usage", OnRequest: true}, fmt.Sprintf(`SELECT count(*), sum(last_seq - recent_seq) FROM sessions WHERE recent_seq < last_seq AND updated_at >= %d`, recent)},
		{Lag{Name: "activity", OnRequest: true}, `SELECT count(*), sum(last_seq - activity_seq) FROM sessions WHERE activity_seq < last_seq`},
		{Lag{Name: "commits and their usage", OnRequest: true}, `SELECT count(*), sum(last_seq - work_seq) FROM sessions WHERE work_seq < last_seq`},
	}
	out := make([]Lag, 0, len(counts))
	for _, c := range counts {
		var events sql.NullInt64
		if err := s.db.QueryRowContext(ctx, c.query).Scan(&c.lag.Sessions, &events); err != nil {
			return nil, fmt.Errorf("store: behind: %w", err)
		}
		c.lag.Events = events.Int64
		out = append(out, c.lag)
	}
	return out, nil
}
