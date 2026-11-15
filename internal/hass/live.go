package hass

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Live keeps a WebSocket open to Home Assistant and reports every state
// change. It reconnects on its own.
type Live struct {
	URL   string // from WebSocketURL
	Token string

	// OnSnapshot gets every entity once after each (re)connect.
	OnSnapshot func([]State)
	// OnChange gets one entity when it changes; s is nil when it was removed.
	OnChange func(id string, s *State)
	// OnStatus is told when the connection comes up or goes down.
	OnStatus func(connected bool, err error)
}

// Run blocks until ctx is cancelled.
func (l *Live) Run(ctx context.Context) {
	wait := time.Second
	for ctx.Err() == nil {
		started := time.Now()
		err := l.session(ctx)
		if ctx.Err() != nil {
			return
		}
		l.OnStatus(false, err)
		// a session that lasted a while earns a quick retry
		if time.Since(started) > time.Minute {
			wait = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if wait *= 2; wait > 30*time.Second {
			wait = 30 * time.Second
		}
	}
}

type wsMessage struct {
	ID      int             `json:"id"`
	Type    string          `json:"type"`
	Success *bool           `json:"success"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
	Event   *struct {
		Data struct {
			EntityID string `json:"entity_id"`
			NewState *State `json:"new_state"`
		} `json:"data"`
	} `json:"event"`
}

const (
	idStates    = 1
	idSubscribe = 2
	readTimeout = 75 * time.Second
	pingEvery   = 30 * time.Second
)

func (l *Live) session(ctx context.Context) error {
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.DialContext(ctx, l.URL, nil)
	if err != nil {
		return fmt.Errorf("couldn't reach Home Assistant: %w", err)
	}
	defer conn.Close()
	conn.SetReadLimit(128 << 20) // get_states on a big install is several MB

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { // unblock the read loop when we are told to stop
		<-ctx.Done()
		conn.Close()
	}()

	var wmu sync.Mutex
	send := func(v any) error {
		wmu.Lock()
		defer wmu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		return conn.WriteJSON(v)
	}
	read := func() (wsMessage, error) {
		var m wsMessage
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		err := conn.ReadJSON(&m)
		return m, err
	}

	m, err := read()
	if err != nil {
		return err
	}
	if m.Type != "auth_required" {
		return fmt.Errorf("unexpected greeting from Home Assistant: %s", m.Type)
	}
	if err := send(map[string]string{"type": "auth", "access_token": l.Token}); err != nil {
		return err
	}
	if m, err = read(); err != nil {
		return err
	}
	switch m.Type {
	case "auth_ok":
	case "auth_invalid":
		return ErrUnauthorized
	default:
		return fmt.Errorf("unexpected answer to sign-in: %s", m.Type)
	}

	if err := send(map[string]any{"id": idStates, "type": "get_states"}); err != nil {
		return err
	}
	if err := send(map[string]any{"id": idSubscribe, "type": "subscribe_events", "event_type": "state_changed"}); err != nil {
		return err
	}

	go func() {
		t := time.NewTicker(pingEvery)
		defer t.Stop()
		id := idSubscribe
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				id++
				if send(map[string]any{"id": id, "type": "ping"}) != nil {
					return
				}
			}
		}
	}()

	for {
		m, err := read()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("lost the connection to Home Assistant")
		}
		switch {
		case m.Type == "result" && m.ID == idStates:
			if m.Success != nil && !*m.Success {
				return errors.New("Home Assistant wouldn't list its entities")
			}
			var states []State
			if err := json.Unmarshal(m.Result, &states); err != nil {
				return err
			}
			l.OnSnapshot(states)
			l.OnStatus(true, nil)
		case m.Type == "event" && m.Event != nil && m.Event.Data.EntityID != "":
			l.OnChange(m.Event.Data.EntityID, m.Event.Data.NewState)
		}
	}
}
