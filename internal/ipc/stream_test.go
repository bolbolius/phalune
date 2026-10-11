package ipc

import (
	"bufio"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func startTestServer(t *testing.T, handler Handler) (*Server, *EventBus, *WidgetHub, string) {
	t.Helper()
	bus := NewEventBus()
	widgets := NewWidgetHub()
	srv, err := NewServer(t.TempDir()+"/test.sock", handler, bus, widgets)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv, bus, widgets, srv.socketPath
}

func dial(t *testing.T, path string) (net.Conn, *bufio.Scanner) {
	t.Helper()
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, bufio.NewScanner(c)
}

func sendLine(t *testing.T, c net.Conn, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := c.Write(append(data, '\n')); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}

func scanLine(t *testing.T, sc *bufio.Scanner) map[string]any {
	t.Helper()
	if !sc.Scan() {
		t.Fatalf("no line from server: %v", sc.Err())
	}
	var m map[string]any
	if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
		t.Fatalf("bad JSON line %q: %v", sc.Text(), err)
	}
	return m
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestSubscribeStreamsEvents(t *testing.T) {
	_, bus, _, path := startTestServer(t, func(req Request) Response {
		return Response{OK: true}
	})

	c, sc := dial(t, path)
	sendLine(t, c, Request{Action: ActionSubscribeEvents, Args: map[string]string{"events": "workspaces,volume"}})

	handshake := scanLine(t, sc)
	if handshake["ok"] != true {
		t.Fatalf("bad handshake: %v", handshake)
	}

	bus.Publish("workspaces", map[string]any{"id": 3})
	bus.Publish("volume", map[string]any{"volume": 72})
	bus.Publish("mpris", map[string]any{"title": "filtered-out"})

	if ev := scanLine(t, sc); ev["event"] != "workspaces" {
		t.Fatalf("expected workspaces event, got %v", ev)
	}
	if ev := scanLine(t, sc); ev["event"] != "volume" {
		t.Fatalf("expected volume event, got %v", ev)
	}
}

func TestSubscribeReplaysLatestState(t *testing.T) {
	_, bus, _, path := startTestServer(t, func(req Request) Response {
		return Response{OK: true}
	})

	bus.Publish("mpris", map[string]any{"title": "current song"})
	bus.Publish("volume", map[string]any{"volume": 10})

	c, sc := dial(t, path)
	sendLine(t, c, Request{Action: ActionSubscribeEvents, Args: map[string]string{"events": "mpris"}})

	handshake := scanLine(t, sc)
	if handshake["ok"] != true {
		t.Fatalf("bad handshake: %v", handshake)
	}
	if ev := scanLine(t, sc); ev["event"] != "mpris" {
		t.Fatalf("expected replayed mpris event, got %v", ev)
	}
}

func TestSubscribeWildcard(t *testing.T) {
	_, bus, _, path := startTestServer(t, nil)

	c, sc := dial(t, path)
	sendLine(t, c, Request{Action: ActionSubscribeEvents})
	scanLine(t, sc)

	bus.Publish("any.topic", "hello")
	if ev := scanLine(t, sc); ev["event"] != "any.topic" {
		t.Fatalf("expected wildcard delivery, got %v", ev)
	}
}

func TestSubscribePatternWildcard(t *testing.T) {
	_, bus, _, path := startTestServer(t, nil)

	c, sc := dial(t, path)
	sendLine(t, c, Request{Action: ActionSubscribeEvents, Args: map[string]string{"events": "mpris.*"}})
	scanLine(t, sc)

	bus.Publish("mpris.track", "song")
	if ev := scanLine(t, sc); ev["event"] != "mpris.track" {
		t.Fatalf("expected pattern match delivery, got %v", ev)
	}
}

func TestSubscribeUnknownActionStillWorks(t *testing.T) {
	_, _, _, path := startTestServer(t, func(req Request) Response {
		return Response{OK: true, Message: "pong"}
	})

	c, sc := dial(t, path)
	sendLine(t, c, Request{Action: ActionPing})
	if resp := scanLine(t, sc); resp["ok"] != true {
		t.Fatalf("ping broke: %v", resp)
	}
}

func TestWidgetWatchPushAndClick(t *testing.T) {
	_, _, widgets, path := startTestServer(t, nil)

	c, sc := dial(t, path)
	sendLine(t, c, Request{Action: ActionWidgetWatch, Args: map[string]string{"id": "pomodoro"}})
	scanLine(t, sc) // handshake

	// Push state over the same connection (bi-directional socket).
	sendLine(t, c, map[string]any{"id": "pomodoro", "text": "18:42", "class": "running", "percentage": 65})

	waitFor(t, func() bool {
		st, ok := widgets.State("pomodoro")
		return ok && st.Text == "18:42"
	}, "pushed state not applied")

	st, _ := widgets.State("pomodoro")
	if st.Percent == nil || *st.Percent != 65 {
		t.Fatalf("percentage mismatch: %+v", st)
	}
	if !st.Visible {
		t.Fatal("push should mark widget visible")
	}

	widgets.DispatchClick(&WidgetEvent{Event: "widget_clicked", ID: "pomodoro", Button: 1})
	if ev := scanLine(t, sc); ev["event"] != "widget_clicked" || ev["id"] != "pomodoro" {
		t.Fatalf("expected click event back on socket, got %v", ev)
	}

	// Empty text clears.
	sendLine(t, c, map[string]any{"id": "pomodoro", "text": ""})
	waitFor(t, func() bool {
		st, _ := widgets.State("pomodoro")
		return !st.Visible
	}, "clear did not hide widget")
}

func TestWidgetOwnershipExclusive(t *testing.T) {
	_, _, _, path := startTestServer(t, nil)

	c1, sc1 := dial(t, path)
	sendLine(t, c1, Request{Action: ActionWidgetWatch, Args: map[string]string{"id": "timer"}})
	scanLine(t, sc1)

	c2, sc2 := dial(t, path)
	sendLine(t, c2, Request{Action: ActionWidgetWatch, Args: map[string]string{"id": "timer"}})
	if resp := scanLine(t, sc2); resp["ok"] != false {
		t.Fatalf("second owner should fail, got %v", resp)
	}
}

func TestWidgetOwnerDisconnectKeepsStateAndReleasesOwnership(t *testing.T) {
	_, _, widgets, path := startTestServer(t, nil)

	c1, sc1 := dial(t, path)
	sendLine(t, c1, Request{Action: ActionWidgetWatch, Args: map[string]string{"id": "net"}})
	scanLine(t, sc1)
	sendLine(t, c1, map[string]any{"id": "net", "text": "up"})

	waitFor(t, func() bool {
		st, ok := widgets.State("net")
		return ok && st.Text == "up"
	}, "pushed state not applied")

	_ = c1.Close()

	waitFor(t, func() bool {
		widgets.mu.RLock()
		_, owned := widgets.owners["net"]
		widgets.mu.RUnlock()
		return !owned
	}, "ownership not released after disconnect")

	if st, ok := widgets.State("net"); !ok || st.Text != "up" {
		t.Fatal("state should persist after owner disconnect")
	}

	// New daemon re-attaches and receives persisted state in handshake.
	c2, sc2 := dial(t, path)
	sendLine(t, c2, Request{Action: ActionWidgetWatch, Args: map[string]string{"id": "net"}})
	h := scanLine(t, sc2)
	if h["ok"] != true {
		t.Fatalf("reattach failed: %v", h)
	}
	data, ok := h["data"].(map[string]any)
	if !ok {
		t.Fatalf("handshake data missing: %v", h)
	}
	state, ok := data["state"].(map[string]any)
	if !ok {
		t.Fatalf("handshake state missing: %v", data)
	}
	if state["text"] != "up" {
		t.Fatalf("handshake should carry persisted state, got %v", state)
	}
}

func TestWidgetStateClampsPercentage(t *testing.T) {
	hub := NewWidgetHub()
	pct := 150.0
	hub.Push(WidgetState{ID: "x", Text: "t", Percent: &pct})
	st, _ := hub.State("x")
	if st.Percent == nil || *st.Percent != 100 {
		t.Fatalf("percentage not clamped: %+v", st)
	}
}

func TestWidgetHubClearNotifiesWatchers(t *testing.T) {
	hub := NewWidgetHub()
	called := false
	unsub := hub.OnChange("test", func() {
		called = true
	})
	defer unsub()

	hub.Push(WidgetState{ID: "test", Text: "active"})
	called = false
	hub.Clear("test")
	if !called {
		t.Fatal("Clear did not notify watcher")
	}
	st, ok := hub.State("test")
	if !ok || st.Visible {
		t.Fatalf("expected hidden state after Clear, got visible=%v", st.Visible)
	}
}

func TestEventBusSlowSubscriberSkips(t *testing.T) {
	bus := NewEventBus()
	ch, unsub := bus.Subscribe(nil)
	defer unsub()

	for i := 0; i < eventBuffer+8; i++ {
		bus.Publish("t", i)
	}
	if len(ch) != eventBuffer {
		t.Fatalf("expected a full buffer, got %d", len(ch))
	}
}

func TestMatchTopic(t *testing.T) {
	cases := []struct {
		topic, filter string
		want          bool
	}{
		{"workspaces", "", true},
		{"workspaces", "*", true},
		{"workspaces", "workspaces", true},
		{"workspaces", "volume", false},
		{"mpris.track", "mpris.*", true},
		{"mpris", "mpris.*", false},
	}
	for _, tc := range cases {
		if got := MatchTopic(tc.topic, tc.filter); got != tc.want {
			t.Errorf("MatchTopic(%q, %q) = %v, want %v", tc.topic, tc.filter, got, tc.want)
		}
	}
}

func TestValidWidgetID(t *testing.T) {
	for _, ok := range []string{"pomodoro", "vpn-1", "a.b_c"} {
		if !ValidWidgetID(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "with space", "up/erp", "colon:", string(make([]byte, 65))} {
		if ValidWidgetID(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestWidgetWatchClearOnDisconnect(t *testing.T) {
	_, _, widgets, path := startTestServer(t, nil)

	c, sc := dial(t, path)
	sendLine(t, c, Request{Action: ActionWidgetWatch, Args: map[string]string{
		"id":                  "demo",
		"clear_on_disconnect": "true",
	}})
	scanLine(t, sc) // handshake
	sendLine(t, c, map[string]any{"id": "demo", "text": "running"})

	waitFor(t, func() bool {
		st, ok := widgets.State("demo")
		return ok && st.Visible && st.Text == "running"
	}, "widget did not become visible")

	_ = c.Close()

	waitFor(t, func() bool {
		st, ok := widgets.State("demo")
		return ok && !st.Visible
	}, "widget was not cleared on disconnect")
}
