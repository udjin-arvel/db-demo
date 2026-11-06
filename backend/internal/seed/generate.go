package seed

import (
	"fmt"
	"math/rand"
	"time"
)

var regions = []string{"eu", "us", "asia", "latam", "africa"}
var segments = []string{"free", "premium", "enterprise"}
var eventTypes = []string{"click", "view", "purchase", "signup", "logout"}

const day = 24 * time.Hour

// UserRow is one user record.
type UserRow struct {
	ID      int64
	Region  string
	Segment string
}

// EventRow is one event record.
type EventRow struct {
	ID        int64
	UserID    int64
	EventType string
	CreatedAt time.Time
	Value     string // decimal text
	Payload   string
}

// GenerateUsers produces deterministic users 1..nUsers.
func GenerateUsers(nUsers int64) []UserRow {
	out := make([]UserRow, 0, nUsers)
	for id := int64(1); id <= nUsers; id++ {
		out = append(out, UserRow{
			ID:      id,
			Region:  regions[id%int64(len(regions))],
			Segment: segments[(id/7)%int64(len(segments))],
		})
	}
	return out
}

// GenEventsBatch fills rows with deterministic events for ids startID..startID+len(rows)-1.
func GenEventsBatch(rows []EventRow, startID, nUsers int64, rng *rand.Rand) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	span := t1.Sub(t0)
	off := startID - 1
	for i := range rows {
		id := off + int64(i) + 1
		u := 1 + rng.Int63n(nUsers)
		et := eventTypes[rng.Intn(len(eventTypes))]
		ts := t0.Add(time.Duration(rng.Int63n(int64(span))))
		v := 0.01 + rng.Float64()*9999.99
		val := fmt.Sprintf("%.2f", v)
		pl := randomPayload(100+rng.Intn(101), rng)
		rows[i] = EventRow{
			ID:        id,
			UserID:    u,
			EventType: et,
			CreatedAt: ts,
			Value:     val,
			Payload:   pl,
		}
	}
}

func randomPayload(n int, rng *rand.Rand) string {
	const alpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 "
	b := make([]byte, n)
	for i := range b {
		b[i] = alpha[rng.Intn(len(alpha))]
	}
	return string(b)
}
