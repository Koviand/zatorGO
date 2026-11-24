package webapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	eventsBufSize     = 64
	eventsDefaultWait = 25 * time.Second
	eventsMaxWait     = 55 * time.Second
)

// Event — событие прогресса для long-poll /api/events (RM0b).
type Event struct {
	Type    string `json:"type"`            // supersweep|update|relock|repair|service|journal
	State   string `json:"state,omitempty"` // starting|running|done|fail|…
	Profile int    `json:"profile,omitempty"`
	Message string `json:"message,omitempty"`
	TS      int64  `json:"ts"` // unix seconds
}

type eventHub struct {
	mu      sync.Mutex
	seq     uint64
	buf     []eventItem
	waiters []chan struct{}
}

type eventItem struct {
	seq uint64
	ev  Event
}

func newEventHub() *eventHub {
	return &eventHub{buf: make([]eventItem, 0, eventsBufSize)}
}

func (h *eventHub) publish(ev Event) {
	if ev.TS == 0 {
		ev.TS = time.Now().Unix()
	}
	h.mu.Lock()
	h.seq++
	item := eventItem{seq: h.seq, ev: ev}
	h.buf = append(h.buf, item)
	if len(h.buf) > eventsBufSize {
		h.buf = h.buf[len(h.buf)-eventsBufSize:]
	}
	waiters := h.waiters
	h.waiters = nil
	h.mu.Unlock()
	for _, ch := range waiters {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (h *eventHub) since(after uint64) (items []eventItem, last uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	last = h.seq
	for _, it := range h.buf {
		if it.seq > after {
			items = append(items, it)
		}
	}
	return items, last
}

func (h *eventHub) wait(after uint64, timeout time.Duration) (items []eventItem, last uint64) {
	items, last = h.since(after)
	if len(items) > 0 || timeout <= 0 {
		return items, last
	}
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.waiters = append(h.waiters, ch)
	h.mu.Unlock()

	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-ch:
	case <-t.C:
	}
	return h.since(after)
}

// publishEvent публикует событие (no-op до инициализации hub).
func (s *Server) publishEvent(ev Event) {
	if s == nil {
		return
	}
	if s.events == nil {
		s.events = newEventHub()
	}
	if ev.TS == 0 && s.now != nil {
		ev.TS = s.now().Unix()
	}
	s.events.publish(ev)
}

// GET /api/events — long-poll снимка событий (RM0b).
// Query: after=<seq>, timeout=<sec> (default 25, max 55), type= filter (optional).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, p params) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	if s.events == nil {
		s.events = newEventHub()
	}
	after, _ := strconv.ParseUint(p.Get("after"), 10, 64)
	timeout := eventsDefaultWait
	if raw := p.Get("timeout"); raw != "" {
		sec, err := strconv.Atoi(raw)
		if err != nil || sec < 0 {
			writeErrorCode(w, http.StatusBadRequest, CodeValidation, "Некорректный timeout", "")
			return
		}
		timeout = time.Duration(sec) * time.Second
		if timeout > eventsMaxWait {
			timeout = eventsMaxWait
		}
	}
	filter := p.Get("type")

	// Немедленный ответ, если уже есть события или timeout=0.
	items, last := s.events.wait(after, timeout)
	events := make([]Event, 0, len(items))
	for _, it := range items {
		if filter != "" && it.ev.Type != filter {
			continue
		}
		events = append(events, it.ev)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"mode":   "long-poll",
		"after":  after,
		"seq":    last,
		"events": events,
	})
}

// encodeEvents — helper для тестов.
func encodeEvents(events []Event) string {
	b, _ := json.Marshal(events)
	return string(b)
}
