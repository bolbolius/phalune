package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

// Handler serves a one-shot request. Streaming actions (subscribe, widget
// watch) are owned by the server and never reach the handler.
type Handler func(req Request) Response

const maxLineSize = 1024 * 1024

// conn wraps one client connection. Writes are serialized through the
// mutex so replies and streamed events never interleave mid-line.
type conn struct {
	c      net.Conn
	writer *bufio.Writer
	wmu    sync.Mutex
	// onStreamEnd releases stream resources (bus subscriptions, widget
	// ownership) when the connection dies. Runs once, on first close.
	onStreamEnd func()
	once        sync.Once
	closed      chan struct{}
}

func newConn(c net.Conn) *conn {
	return &conn{
		c:      c,
		writer: bufio.NewWriter(c),
		closed: make(chan struct{}),
	}
}

func (c *conn) writeJSON(v any) bool {
	data, err := json.Marshal(v)
	if err != nil {
		return false
	}

	c.wmu.Lock()
	defer c.wmu.Unlock()
	if _, err := c.writer.Write(append(data, '\n')); err != nil {
		return false
	}
	return c.writer.Flush() == nil
}

func (c *conn) close() {
	c.once.Do(func() {
		close(c.closed)
		if c.onStreamEnd != nil {
			c.onStreamEnd()
		}
	})
	_ = c.c.Close()
}

type Server struct {
	socketPath string
	listener   net.Listener
	handler    Handler
	bus        *EventBus
	widgets    *WidgetHub
	wg         sync.WaitGroup
	closed     chan struct{}
}

func NewServer(socketPath string, handler Handler, bus *EventBus, widgets *WidgetHub) (*Server, error) {
	if socketPath == "" {
		socketPath = DefaultSocketPath()
	}

	_ = os.Remove(socketPath)

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on socket %s: %w", socketPath, err)
	}

	s := &Server{
		socketPath: socketPath,
		listener:   l,
		handler:    handler,
		bus:        bus,
		widgets:    widgets,
		closed:     make(chan struct{}),
	}

	s.wg.Add(1)
	go s.serve()

	return s, nil
}

func (s *Server) serve() {
	defer s.wg.Done()

	for {
		c, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return
			default:
				if errors.Is(err, net.ErrClosed) {
					return
				}
				slog.Debug("ipc: accept error", "error", err)
				continue
			}
		}

		s.wg.Add(1)
		go s.handleConn(newConn(c))
	}
}

// handleConn reads NDJSON requests until the client disconnects. One-shot
// requests answer inline. Streaming requests answer with a handshake and
// keep the connection open: a goroutine pumps server events out while
// this loop keeps reading client lines in, so a single watched socket is
// bi-directional.
func (s *Server) handleConn(c *conn) {
	defer s.wg.Done()
	defer c.close()

	scanner := bufio.NewScanner(c.c)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	for {
		if !scanner.Scan() {
			return
		}

		var req Request
		line := scanner.Bytes()
		if err := json.Unmarshal(line, &req); err != nil {
			req = rawAction(string(line))
		}

		switch req.Action {
		case ActionSubscribeEvents:
			ch, _, ok := s.startEventStream(c, &req)
			if !ok {
				return
			}
			if !c.writeJSON(Response{
				OK:      true,
				Message: "subscribed",
				Data:    mustJSON(map[string]any{"topics": SortedTopics(ParseEventTopics(req.Args["events"]))}),
			}) {
				return
			}
			s.pumpEvents(c, ch)
			return

		case ActionWidgetWatch:
			events, ok := s.startWidgetStream(c, &req)
			if !ok {
				return
			}
			if !c.writeJSON(Response{OK: true, Message: "watching", Data: mustJSON(s.watchHandshake(req.Args["id"]))}) {
				return
			}
			s.wg.Add(1)
			go s.pumpWidgetEvents(c, events)
			s.pumpWidgetPushes(c, scanner, req.Args["id"])
			return

		default:
			if !c.writeJSON(s.dispatch(req)) {
				return
			}
		}
	}
}

func (s *Server) dispatch(req Request) Response {
	if s.handler == nil {
		return Response{Error: "no handler configured"}
	}
	return s.handler(req)
}

func (s *Server) startEventStream(c *conn, req *Request) (<-chan Event, func(), bool) {
	if s.bus == nil {
		c.writeJSON(Response{OK: false, Error: "event bus not available"})
		return nil, nil, false
	}

	ch, unsub := s.bus.Subscribe(ParseEventTopics(req.Args["events"]))

	prev := c.onStreamEnd
	c.onStreamEnd = func() {
		unsub()
		if prev != nil {
			prev()
		}
	}
	return ch, unsub, true
}

func (s *Server) pumpEvents(c *conn, ch <-chan Event) {
	for {
		select {
		case <-c.closed:
			return
		case <-s.closed:
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if !c.writeJSON(eventEnvelope{Event: ev.Topic, Data: ev.Data}) {
				return
			}
		}
	}
}

func (s *Server) startWidgetStream(c *conn, req *Request) (<-chan *WidgetEvent, bool) {
	if s.widgets == nil {
		c.writeJSON(Response{OK: false, Error: "widget registry not available"})
		return nil, false
	}

	id := req.Args["id"]
	if !ValidWidgetID(id) {
		c.writeJSON(Response{OK: false, Error: "invalid or missing widget id"})
		return nil, false
	}

	if !s.widgets.attach(id) {
		c.writeJSON(Response{OK: false, Error: fmt.Sprintf("widget %q already owned by another connection", id)})
		return nil, false
	}

	clearOnDisconnect := req.Args["clear_on_disconnect"] == "true"
	prev := c.onStreamEnd
	c.onStreamEnd = func() {
		s.widgets.detach(id)
		if clearOnDisconnect {
			s.widgets.Clear(id)
		}
		if prev != nil {
			prev()
		}
	}
	return s.widgets.events(id), true
}

func (s *Server) pumpWidgetEvents(c *conn, events <-chan *WidgetEvent) {
	defer s.wg.Done()

	for {
		select {
		case <-c.closed:
			return
		case <-s.closed:
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if !c.writeJSON(ev) {
				return
			}
		}
	}
}

// pumpWidgetPushes consumes widget state lines pushed by the owning
// connection, updating the hub the bar widget renders from. An empty
// text (and icon) clears the widget.
func (s *Server) pumpWidgetPushes(c *conn, scanner *bufio.Scanner, id string) {
	for {
		if !scanner.Scan() {
			return
		}

		var st WidgetState
		if err := json.Unmarshal(scanner.Bytes(), &st); err != nil {
			slog.Debug("ipc: invalid widget push", "widget", id, "error", err)
			continue
		}
		if st.ID == "" {
			st.ID = id
		}
		if st.ID != id {
			c.writeJSON(Response{OK: false, Error: "push id mismatch"})
			continue
		}
		if st.Text == "" && st.Icon == "" {
			s.widgets.Clear(id)
			continue
		}
		s.widgets.Push(st)
	}
}

// watchHandshake reports the persisted state of the watched widget so a
// reconnecting daemon can sync.
func (s *Server) watchHandshake(id string) map[string]any {
	state := WidgetState{ID: id}
	if st, ok := s.widgets.State(id); ok {
		state = st
	}
	return map[string]any{"id": id, "state": state}
}

// Close shuts the listener down, drops all clients, and removes the socket.
func (s *Server) Close() error {
	select {
	case <-s.closed:
		return nil
	default:
		close(s.closed)
	}

	var err error
	if s.listener != nil {
		err = s.listener.Close()
	}
	_ = os.Remove(s.socketPath)

	if s.widgets != nil {
		s.widgets.closeAll()
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}

	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}
