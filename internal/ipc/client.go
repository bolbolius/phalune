package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

// Client is a connection to the running shell. One-shot requests use
// SendCommand; long-lived consumers use Dial and stream.
type Client struct {
	conn   net.Conn
	writer *bufio.Writer
	reader *bufio.Scanner
}

func Dial(socketPath string) (*Client, error) {
	if socketPath == "" {
		socketPath = DefaultSocketPath()
	}
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("could not connect to phalune socket at %s: %w", socketPath, err)
	}
	return &Client{
		conn:   conn,
		writer: bufio.NewWriter(conn),
		reader: bufio.NewScanner(conn),
	}, nil
}

// Request writes one NDJSON request line.
func (c *Client) Request(req Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}
	if _, err := c.writer.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	return c.writer.Flush()
}

// RawLine writes one raw NDJSON line (widget pushes from a daemon).
func (c *Client) RawLine(line []byte) error {
	if _, err := c.writer.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("failed to send line: %w", err)
	}
	return c.writer.Flush()
}

// Response reads one NDJSON reply line.
func (c *Client) Response() (*Response, error) {
	if !c.reader.Scan() {
		if err := c.reader.Err(); err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		return nil, fmt.Errorf("connection closed by server")
	}
	var resp Response
	if err := json.Unmarshal(c.reader.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

// Close ends the connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// SendCommand performs one one-shot request/response round trip.
func SendCommand(socketPath, action string, args map[string]string) (*Response, error) {
	client, err := Dial(socketPath)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	if err := client.Request(Request{Action: action, Args: args}); err != nil {
		return nil, err
	}
	return client.Response()
}

// PushWidget sends a one-shot widget state push (widget-push action).
func PushWidget(socketPath string, st WidgetState) (*Response, error) {
	if !ValidWidgetID(st.ID) {
		return nil, fmt.Errorf("invalid widget id %q", st.ID)
	}
	data, err := json.Marshal(st)
	if err != nil {
		return nil, err
	}
	return SendCommand(socketPath, ActionWidgetPush, map[string]string{
		"id":   st.ID,
		"json": string(data),
	})
}

// ClearWidget sends a one-shot widget clear.
func ClearWidget(socketPath, id string) (*Response, error) {
	if !ValidWidgetID(id) {
		return nil, fmt.Errorf("invalid widget id %q", id)
	}
	return SendCommand(socketPath, ActionWidgetClear, map[string]string{"id": id})
}

// StreamEvents subscribes and prints matching events as NDJSON on stdout
// until SIGINT/SIGTERM. Pattern matching happens client-side on the wire
// envelopes.
func StreamEvents(socketPath string, patterns []string) error {
	client, err := Dial(socketPath)
	if err != nil {
		return err
	}
	defer client.Close()

	req := Request{Action: ActionSubscribeEvents}
	if len(patterns) > 0 {
		req.Args = map[string]string{"events": strings.Join(patterns, ",")}
	}
	if err := client.Request(req); err != nil {
		return err
	}

	resp, err := client.Response()
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}

	// Ctrl-C ends the stream promptly instead of leaving ^C half-printed.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	stopSig := make(chan struct{})
	defer func() {
		signal.Stop(sigCh)
		close(stopSig)
	}()
	go func() {
		select {
		case <-sigCh:
			client.Close()
		case <-stopSig:
		}
	}()

	for client.reader.Scan() {
		line := client.reader.Bytes()
		var env eventEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			fmt.Println(string(line))
			continue
		}
		matched := len(patterns) == 0
		for _, p := range patterns {
			if MatchTopic(env.Event, p) {
				matched = true
				break
			}
		}
		if matched {
			fmt.Println(string(line))
		}
	}
	return nil
}

// WatchWidget claims a widget id over a persistent connection: it pushes
// state from stdin (NDJSON, one state per line) and prints interaction
// events received from the shell. Interrupt ends the watch.
func WatchWidget(socketPath, id string) error {
	if !ValidWidgetID(id) {
		return fmt.Errorf("invalid widget id %q", id)
	}

	client, err := Dial(socketPath)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Request(Request{Action: ActionWidgetWatch, Args: map[string]string{"id": id}}); err != nil {
		return err
	}

	resp, err := client.Response()
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}

	// Ctrl-C: release the widget cleanly.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	stopSig := make(chan struct{})
	defer func() {
		signal.Stop(sigCh)
		close(stopSig)
	}()
	go func() {
		select {
		case <-sigCh:
			client.Close()
			os.Exit(0)
		case <-stopSig:
		}
	}()

	go func() {
		for client.reader.Scan() {
			fmt.Println(string(client.reader.Bytes()))
		}
	}()

	stdin := bufio.NewScanner(os.Stdin)
	stdin.Buffer(make([]byte, 0, 64*1024), maxLineSize)
	for stdin.Scan() {
		line := strings.TrimSpace(stdin.Text())
		if line == "" || line == "clear" {
			if err := client.RawLine([]byte(`{"id":"` + id + `"}`)); err != nil {
				return nil
			}
			continue
		}
		if !json.Valid([]byte(line)) {
			fmt.Fprintf(os.Stderr, "invalid JSON: %s\n", line)
			continue
		}
		if err := client.RawLine([]byte(line)); err != nil {
			return nil
		}
	}
	return nil
}

// parseFloatArg parses a percentage flag value.
func parseFloatArg(raw string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
