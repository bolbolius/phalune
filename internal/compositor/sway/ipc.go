package sway

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
)

const swayMagic = "i3-ipc"

const (
	msgRunCommand    = 0
	msgGetWorkspaces = 1
	msgSubscribe     = 2
	msgGetOutputs    = 3
	msgGetTree       = 4
	msgGetVersion    = 7
)

const (
	eventWorkspace = "workspace"
	eventWindow    = "window"
)

type conn struct {
	c net.Conn
}

func dial(socketPath string) (*conn, error) {
	path := socketPath
	if path == "" {
		path = os.Getenv("SWAYSOCK")
		if path == "" {
			return nil, fmt.Errorf("SWAYSOCK not set")
		}
	}
	c, err := net.Dial("unix", path)
	if err != nil {
		return nil, fmt.Errorf("connect sway ipc %q: %w", path, err)
	}
	return &conn{c: c}, nil
}

func (c *conn) Close() error {
	if c == nil || c.c == nil {
		return nil
	}
	return c.c.Close()
}

func (c *conn) raw(msgType uint32, payload string) ([]byte, error) {
	if c == nil || c.c == nil {
		return nil, fmt.Errorf("sway ipc not connected")
	}

	pl := []byte(payload)
	headerLen := 14
	buf := make([]byte, 0, headerLen+len(pl))
	buf = append(buf, swayMagic...)
	var lbuf, tbuf [4]byte
	binary.LittleEndian.PutUint32(lbuf[:], uint32(len(pl)))
	binary.LittleEndian.PutUint32(tbuf[:], msgType)
	buf = append(buf, lbuf[:]...)
	buf = append(buf, tbuf[:]...)
	buf = append(buf, pl...)

	if _, err := c.c.Write(buf); err != nil {
		return nil, fmt.Errorf("sway ipc write: %w", err)
	}

	hdr := make([]byte, headerLen)
	if _, err := ioReadFull(c.c, hdr); err != nil {
		return nil, fmt.Errorf("sway ipc read header: %w", err)
	}
	if !bytes.Equal(hdr[:6], []byte(swayMagic)) {
		return nil, fmt.Errorf("sway ipc unexpected reply magic")
	}
	replyLen := binary.LittleEndian.Uint32(hdr[6:10])
	replyType := binary.LittleEndian.Uint32(hdr[10:14])

	body := make([]byte, replyLen)
	if _, err := ioReadFull(c.c, body); err != nil {
		return nil, fmt.Errorf("sway ipc read body: %w", err)
	}

	if replyType&0x80000000 != 0 {
		return nil, fmt.Errorf("sway ipc unexpected event reply")
	}

	return body, nil
}

func ioReadFull(c net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := c.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

type eventConn struct {
	c    *conn
	ch   chan event
	errc chan error
	stop chan struct{}
	once sync.Once
}

type event struct {
	Change    string          `json:"change"`
	Current   json.RawMessage `json:"current"`
	Container json.RawMessage `json:"container"`
}

func (c *conn) events(names ...string) (*eventConn, error) {
	pl, err := json.Marshal(names)
	if err != nil {
		return nil, err
	}

	if _, err := c.raw(msgSubscribe, string(pl)); err != nil {
		return nil, fmt.Errorf("sway ipc subscribe: %w", err)
	}

	ec := &eventConn{
		c:    c,
		ch:   make(chan event, 16),
		errc: make(chan error, 1),
		stop: make(chan struct{}),
	}
	go ec.readLoop()
	return ec, nil
}

func (ec *eventConn) readLoop() {
	defer close(ec.ch)
	for {
		select {
		case <-ec.stop:
			return
		default:
		}

		hdr := make([]byte, 14)
		if _, err := ioReadFull(ec.c.c, hdr); err != nil {
			select {
			case ec.errc <- err:
			default:
			}
			return
		}
		if !bytes.Equal(hdr[:6], []byte(swayMagic)) {
			select {
			case ec.errc <- fmt.Errorf("sway ipc event stream: bad magic"):
			default:
			}
			return
		}
		replyLen := binary.LittleEndian.Uint32(hdr[6:10])
		replyType := binary.LittleEndian.Uint32(hdr[10:14])

		body := make([]byte, replyLen)
		if _, err := ioReadFull(ec.c.c, body); err != nil {
			select {
			case ec.errc <- err:
			default:
			}
			return
		}

		if replyType&0x80000000 == 0 {
			continue
		}

		var ev event
		if err := json.Unmarshal(body, &ev); err != nil {
			continue
		}
		select {
		case ec.ch <- ev:
		default:
			select {
			case <-ec.ch:
			default:
			}
			select {
			case ec.ch <- ev:
			default:
			}
		}
	}
}

func (ec *eventConn) close() {
	ec.once.Do(func() {
		close(ec.stop)
		_ = ec.c.Close()
	})
}
