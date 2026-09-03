package live

import (
	"encoding/json"
	"sync"
)

// EventPayload — сообщение для SSE/WebSocket клиентов.
type EventPayload struct {
	EventID    int64                  `json:"event_id"`
	DeviceID   int64                  `json:"device_id"`
	DeviceName string                 `json:"device_name,omitempty"`
	DeviceHost string                 `json:"device_host,omitempty"`
	IfIndex    *int                   `json:"if_index,omitempty"`
	EventType  string                 `json:"event_type"`
	Severity   string                 `json:"severity"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
}

// Hub рассылает события подписчикам SSE.
type Hub struct {
	mu   sync.RWMutex
	subs map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[chan []byte]struct{})}
}

func (h *Hub) Subscribe() chan []byte {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *Hub) Publish(ev EventPayload) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
		}
	}
}
