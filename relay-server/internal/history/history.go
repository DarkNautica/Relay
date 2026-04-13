package history

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Event is a single stored event in the history buffer.
type Event struct {
	ID        int64           `json:"id"`
	Timestamp string          `json:"timestamp"`
	Channel   string          `json:"channel"`
	EventName string          `json:"event"`
	Data      json.RawMessage `json:"data"`
	SocketID  string          `json:"socket_id,omitempty"`
}

// channelBuffer is a ring buffer of events for one channel.
type channelBuffer struct {
	events []Event
	pos    int
	len    int
	cap    int
}

func newChannelBuffer(capacity int) *channelBuffer {
	return &channelBuffer{
		events: make([]Event, capacity),
		cap:    capacity,
	}
}

func (b *channelBuffer) push(e Event) {
	b.events[b.pos] = e
	b.pos = (b.pos + 1) % b.cap
	if b.len < b.cap {
		b.len++
	}
}

// newest returns up to n events newest-first.
func (b *channelBuffer) newest(n int) []Event {
	if n > b.len {
		n = b.len
	}
	result := make([]Event, n)
	for i := 0; i < n; i++ {
		idx := (b.pos - 1 - i + b.cap) % b.cap
		result[i] = b.events[idx]
	}
	return result
}

// afterID returns events with ID > afterID, oldest-first, up to limit.
func (b *channelBuffer) afterID(afterID int64, limit int) []Event {
	// Collect matching events oldest-first
	all := make([]Event, 0, b.len)
	for i := b.len - 1; i >= 0; i-- {
		idx := (b.pos - 1 - i + b.cap) % b.cap
		if b.events[idx].ID > afterID {
			all = append(all, b.events[idx])
		}
	}
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all
}

// beforeID returns up to limit events with ID < beforeID, newest-first.
func (b *channelBuffer) beforeID(beforeID int64, limit int) []Event {
	result := make([]Event, 0, limit)
	for i := 0; i < b.len && len(result) < limit; i++ {
		idx := (b.pos - 1 - i + b.cap) % b.cap
		e := b.events[idx]
		if e.ID > 0 && e.ID < beforeID {
			result = append(result, e)
		}
	}
	return result
}

// expireBefore removes entries older than the given time.
func (b *channelBuffer) expireBefore(cutoff time.Time) {
	// Walk from oldest to newest and zero out expired entries
	newLen := 0
	for i := b.len - 1; i >= 0; i-- {
		idx := (b.pos - 1 - i + b.cap) % b.cap
		t, err := time.Parse(time.RFC3339, b.events[idx].Timestamp)
		if err != nil || t.Before(cutoff) {
			b.events[idx] = Event{}
		} else {
			newLen++
		}
	}
	// Rebuild compacted buffer
	if newLen < b.len {
		valid := make([]Event, 0, newLen)
		for i := b.len - 1; i >= 0; i-- {
			idx := (b.pos - 1 - i + b.cap) % b.cap
			if b.events[idx].ID != 0 {
				valid = append(valid, b.events[idx])
			}
		}
		b.events = make([]Event, b.cap)
		b.pos = 0
		b.len = 0
		for _, e := range valid {
			b.push(e)
		}
	}
}

// Store is an in-memory event history store with per-channel ring buffers.
type Store struct {
	mu       sync.RWMutex
	buffers  map[string]*channelBuffer // key: appID + "\x00" + channelName
	nextID   atomic.Int64
	defLimit int
}

// NewStore creates a history store and starts background cleanup.
func NewStore(defaultLimit int) *Store {
	s := &Store{
		buffers:  make(map[string]*channelBuffer),
		defLimit: defaultLimit,
	}
	go s.cleanup()
	return s
}

func bufferKey(appID, channel string) string {
	return appID + "\x00" + channel
}

// Record stores an event and returns its sequential ID.
func (s *Store) Record(appID, channel, eventName string, data json.RawMessage, socketID string, limit int) int64 {
	id := s.nextID.Add(1)
	e := Event{
		ID:        id,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Channel:   channel,
		EventName: eventName,
		Data:      data,
		SocketID:  socketID,
	}

	key := bufferKey(appID, channel)
	s.mu.Lock()
	buf, ok := s.buffers[key]
	if !ok {
		cap := limit
		if cap <= 0 {
			cap = s.defLimit
		}
		buf = newChannelBuffer(cap)
		s.buffers[key] = buf
	}
	buf.push(e)
	s.mu.Unlock()

	return id
}

// GetNewest returns the newest n events for a channel, newest-first.
func (s *Store) GetNewest(appID, channel string, n int) []Event {
	key := bufferKey(appID, channel)
	s.mu.RLock()
	buf, ok := s.buffers[key]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	return buf.newest(n)
}

// GetAfterID returns events with ID > afterID for replay, oldest-first.
func (s *Store) GetAfterID(appID, channel string, afterID int64, limit int) []Event {
	key := bufferKey(appID, channel)
	s.mu.RLock()
	buf, ok := s.buffers[key]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	return buf.afterID(afterID, limit)
}

// GetBeforeID returns up to limit events with ID < beforeID, newest-first.
// Used for cursor-based pagination.
func (s *Store) GetBeforeID(appID, channel string, beforeID int64, limit int) []Event {
	key := bufferKey(appID, channel)
	s.mu.RLock()
	buf, ok := s.buffers[key]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	return buf.beforeID(beforeID, limit)
}

// EncodeCursor encodes an event ID as an opaque pagination cursor.
func EncodeCursor(eventID int64) string {
	return base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", eventID)))
}

// DecodeCursor decodes a pagination cursor back to an event ID.
// Returns 0 if the cursor is invalid.
func DecodeCursor(cursor string) int64 {
	raw, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return 0
	}
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// cleanup expires entries older than 1 hour every minute.
func (s *Store) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-1 * time.Hour)
		s.mu.Lock()
		for key, buf := range s.buffers {
			buf.expireBefore(cutoff)
			if buf.len == 0 {
				delete(s.buffers, key)
			}
		}
		s.mu.Unlock()
	}
}
