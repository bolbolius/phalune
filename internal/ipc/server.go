package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

type Handler func(req Request) Response

type Server struct {
	socketPath string
	listener   net.Listener
	handler    Handler
	wg         sync.WaitGroup
	closed     chan struct{}
}

func NewServer(socketPath string, handler Handler) (*Server, error) {
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
		closed:     make(chan struct{}),
	}

	s.wg.Add(1)
	go s.serve()

	return s, nil
}

func (s *Server) serve() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return
			default:
				continue
			}
		}

		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}

	data := scanner.Bytes()
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		req.Action = string(data)
	}

	var resp Response
	if s.handler != nil {
		resp = s.handler(req)
	} else {
		resp = Response{Error: "no handler configured"}
	}

	out, _ := json.Marshal(resp)
	out = append(out, '\n')
	_, _ = conn.Write(out)
}

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

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
	}

	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}
