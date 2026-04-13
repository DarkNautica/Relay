package eventstore

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"
)

// StoredEvent represents a single event tracked by the event store.
type StoredEvent struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	Channel     string    `json:"channel"`
	EventName   string    `json:"event"`
	Data        string    `json:"data"`
	SocketID    string    `json:"socket_id,omitempty"`
	PublishedAt time.Time `json:"published_at"`
	DeliveredTo []string  `json:"delivered_to"`
	DeliveryMs  int64     `json:"delivery_ms"`
}

// AppMetrics contains aggregate metrics for an app.
type AppMetrics struct {
	TotalEvents     int64   `json:"total_events"`
	TotalDeliveries int64   `json:"total_deliveries"`
	AvgLatencyMs    int64   `json:"avg_latency_ms"`
	P50LatencyMs    int64   `json:"p50_latency_ms"`
	P95LatencyMs    int64   `json:"p95_latency_ms"`
	P99LatencyMs    int64   `json:"p99_latency_ms"`
	EventsPerMinute float64 `json:"events_per_minute"`
}

// appData holds the ring buffer and metrics for a single app.
type appData struct {
	events   []StoredEvent
	pos      int
	len      int
	cap      int
	eventIdx map[string]int // event ID -> position in events slice

	// Aggregate counters
	totalEvents     int64
	totalDeliveries int64

	// Sliding window of latency samples for percentile calculation
	latencies    []int64
	latencyPos   int
	latencyLen   int
	latencyCap   int
	latencySum   int64

	// Timestamps for events-per-minute calculation
	eventTimes []time.Time
}

func newAppData(capacity int) *appData {
	return &appData{
		events:     make([]StoredEvent, capacity),
		cap:        capacity,
		eventIdx:   make(map[string]int),
		latencies:  make([]int64, 1000),
		latencyCap: 1000,
		eventTimes: make([]time.Time, 0, 128),
	}
}

// Store is a thread-safe in-memory event store that tracks events per app.
type Store struct {
	mu       sync.RWMutex
	apps     map[string]*appData
	capacity int
}

// NewStore creates an event store with the given per-app capacity.
func NewStore(perAppCapacity int) *Store {
	if perAppCapacity <= 0 {
		perAppCapacity = 1000
	}
	return &Store{
		apps:     make(map[string]*appData),
		capacity: perAppCapacity,
	}
}

// GenerateEventID creates a unique event ID.
func GenerateEventID() string {
	return fmt.Sprintf("evt_%d%06d", time.Now().UnixMicro(), rand.Intn(999999))
}

// getOrCreate returns the appData for an app, creating it if needed.
// Caller must hold the write lock.
func (s *Store) getOrCreate(appID string) *appData {
	ad, ok := s.apps[appID]
	if !ok {
		ad = newAppData(s.capacity)
		s.apps[appID] = ad
	}
	return ad
}

// Add stores an event, evicting the oldest if over capacity.
func (s *Store) Add(event StoredEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ad := s.getOrCreate(event.AppID)

	// If we're overwriting an existing slot, remove old index entry
	if ad.len == ad.cap {
		old := ad.events[ad.pos]
		delete(ad.eventIdx, old.ID)
	}

	ad.events[ad.pos] = event
	ad.eventIdx[event.ID] = ad.pos
	ad.pos = (ad.pos + 1) % ad.cap
	if ad.len < ad.cap {
		ad.len++
	}

	ad.totalEvents++
	ad.eventTimes = append(ad.eventTimes, event.PublishedAt)
}

// Get returns recent events for an app, newest first.
// If beforeID is non-empty, returns events older than that event (cursor pagination).
func (s *Store) Get(appID string, limit int, beforeID string) []StoredEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ad, ok := s.apps[appID]
	if !ok {
		return nil
	}

	if limit <= 0 {
		limit = 25
	}

	// Collect events newest-first
	result := make([]StoredEvent, 0, limit)
	pastCursor := beforeID == ""

	for i := 0; i < ad.len && len(result) < limit; i++ {
		idx := (ad.pos - 1 - i + ad.cap) % ad.cap
		e := ad.events[idx]

		if !pastCursor {
			if e.ID == beforeID {
				pastCursor = true
			}
			continue
		}

		result = append(result, e)
	}

	return result
}

// GetByChannel returns events for a specific channel, newest first.
func (s *Store) GetByChannel(appID, channel string, limit int) []StoredEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ad, ok := s.apps[appID]
	if !ok {
		return nil
	}

	if limit <= 0 {
		limit = 25
	}

	result := make([]StoredEvent, 0, limit)
	for i := 0; i < ad.len && len(result) < limit; i++ {
		idx := (ad.pos - 1 - i + ad.cap) % ad.cap
		e := ad.events[idx]
		if e.Channel == channel {
			result = append(result, e)
		}
	}

	return result
}

// GetByID returns a single event by ID, or nil if not found.
func (s *Store) GetByID(appID, eventID string) *StoredEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ad, ok := s.apps[appID]
	if !ok {
		return nil
	}

	pos, ok := ad.eventIdx[eventID]
	if !ok {
		return nil
	}

	e := ad.events[pos]
	if e.ID != eventID {
		// Index was stale (slot was overwritten)
		return nil
	}
	// Return a copy
	cp := e
	cp.DeliveredTo = make([]string, len(e.DeliveredTo))
	copy(cp.DeliveredTo, e.DeliveredTo)
	return &cp
}

// RecordDelivery records that a socket received an event.
func (s *Store) RecordDelivery(appID, eventID, socketID string, latencyMs int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ad, ok := s.apps[appID]
	if !ok {
		return
	}

	pos, ok := ad.eventIdx[eventID]
	if !ok {
		return
	}

	e := &ad.events[pos]
	if e.ID != eventID {
		return
	}

	e.DeliveredTo = append(e.DeliveredTo, socketID)
	if latencyMs > e.DeliveryMs {
		e.DeliveryMs = latencyMs
	}

	// Update aggregate metrics
	ad.totalDeliveries++

	// Add to latency sliding window
	if ad.latencyLen < ad.latencyCap {
		ad.latencies[ad.latencyLen] = latencyMs
		ad.latencyLen++
	} else {
		old := ad.latencies[ad.latencyPos]
		ad.latencySum -= old
		ad.latencies[ad.latencyPos] = latencyMs
		ad.latencyPos = (ad.latencyPos + 1) % ad.latencyCap
	}
	ad.latencySum += latencyMs
}

// GetMetrics returns aggregate metrics for an app.
func (s *Store) GetMetrics(appID string) AppMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ad, ok := s.apps[appID]
	if !ok {
		return AppMetrics{}
	}

	m := AppMetrics{
		TotalEvents:     ad.totalEvents,
		TotalDeliveries: ad.totalDeliveries,
	}

	// Calculate latency percentiles
	if ad.latencyLen > 0 {
		sorted := make([]int64, ad.latencyLen)
		copy(sorted, ad.latencies[:ad.latencyLen])
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

		m.AvgLatencyMs = ad.latencySum / int64(ad.latencyLen)
		m.P50LatencyMs = percentile(sorted, 50)
		m.P95LatencyMs = percentile(sorted, 95)
		m.P99LatencyMs = percentile(sorted, 99)
	}

	// Calculate events per minute from the last 60 seconds
	now := time.Now()
	cutoff := now.Add(-60 * time.Second)
	count := 0
	for i := len(ad.eventTimes) - 1; i >= 0; i-- {
		if ad.eventTimes[i].Before(cutoff) {
			break
		}
		count++
	}
	m.EventsPerMinute = float64(count)

	return m
}

// percentile returns the p-th percentile from a sorted slice.
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// EncodeCursor encodes an event ID as an opaque pagination cursor.
func EncodeCursor(eventID string) string {
	return base64.URLEncoding.EncodeToString([]byte(eventID))
}

// DecodeCursor decodes a pagination cursor back to an event ID.
func DecodeCursor(cursor string) string {
	raw, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return ""
	}
	return string(raw)
}

// EncodeCursorInt encodes an int64 as a cursor (for backward compat with history).
func EncodeCursorInt(id int64) string {
	return base64.URLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}
