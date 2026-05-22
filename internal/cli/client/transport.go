package client

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/eclipse-iofog/agent/internal/cli/output"
	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

// AttachExecSession attaches to an exec WebSocket session and streams stdin/stdout/stderr.
// Returns the remote command exit code when the session ends cleanly.
func AttachExecSession(c *Client, selector, sessionID string) (int, error) {
	conn, err := DialWS(c, "/v3/ms/"+selector+"/exec/sessions/"+sessionID+":attach")
	if err != nil {
		return 0, fmt.Errorf("attach exec session: %w", err)
	}
	defer conn.Close()

	done := make(chan struct{})
	var doneOnce sync.Once
	exitCode := 0
	resizeDone := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		if oldState, rawErr := term.MakeRaw(int(os.Stdin.Fd())); rawErr == nil {
			defer func() {
				_ = term.Restore(int(os.Stdin.Fd()), oldState)
			}()
		}
	}
	sendResize := func() {
		cols, rows, err := terminalSize()
		if err != nil {
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"type": "resize",
			"cols": cols,
			"rows": rows,
		})
	}
	sendResize()
	go func() {
		defer close(resizeDone)
		for range sigCh {
			sendResize()
		}
	}()
	go func() {
		buf := make([]byte, 1024)
		for {
			n, readErr := os.Stdin.Read(buf)
			data := buf[:0]
			if n > 0 {
				data = buf[:n]
			}
			if len(data) > 0 {
				_ = conn.WriteMessage(websocket.BinaryMessage, data)
			}
			if readErr != nil {
				_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				doneOnce.Do(func() { close(done) })
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			signal.Stop(sigCh)
			close(sigCh)
			<-resizeDone
			return exitCode, nil
		default:
		}
		var event map[string]interface{}
		if err := conn.ReadJSON(&event); err != nil {
			signal.Stop(sigCh)
			close(sigCh)
			<-resizeDone
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return exitCode, nil
			}
			return exitCode, nil
		}
		stream := output.MapValueAsString(event, "stream")
		line := output.MapValueAsRawString(event, "line")
		if stream == "control" {
			if rawCode, ok := event["exitCode"]; ok {
				switch typed := rawCode.(type) {
				case float64:
					exitCode = int(typed)
				case int:
					exitCode = typed
				case string:
					if parsed, parseErr := strconv.Atoi(strings.TrimSpace(typed)); parseErr == nil {
						exitCode = parsed
					}
				}
			}
			continue
		}
		if stream == "stderr" {
			_, _ = io.WriteString(os.Stderr, line)
		} else if stream == "stdout" {
			_, _ = io.WriteString(os.Stdout, line)
		}
	}
}

// StreamMSLogs follows microservice logs over WebSocket.
func StreamMSLogs(c *Client, id, tail, since, until string, timestamps bool) error {
	wsPath := "/v3/ms/" + id + "/logs:stream?tail=" + url.QueryEscape(tail)
	if since != "" {
		wsPath += "&since=" + url.QueryEscape(since)
	}
	if until != "" {
		wsPath += "&until=" + url.QueryEscape(until)
	}
	return streamLogs(c, wsPath, timestamps)
}

// StreamSystemLogs follows daemon logs over WebSocket.
func StreamSystemLogs(c *Client, tail, since, until string, timestamps bool) error {
	wsPath := "/v3/system/logs:stream?tailLines=" + url.QueryEscape(tail)
	if since != "" {
		wsPath += "&since=" + url.QueryEscape(since)
	}
	if until != "" {
		wsPath += "&until=" + url.QueryEscape(until)
	}
	return streamLogs(c, wsPath, timestamps)
}

func streamLogs(c *Client, wsPath string, timestamps bool) error {
	conn, err := DialWS(c, wsPath)
	if err != nil {
		return fmt.Errorf("stream logs: %w", err)
	}
	defer conn.Close()
	for {
		var event map[string]interface{}
		if err := conn.ReadJSON(&event); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return nil
		}
		line := output.MapValueAsRawString(event, "line")
		output.WriteStreamLogLine(os.Stdout, output.MapValueAsString(event, "ts"), line, timestamps)
	}
}

// DialWS opens a LocalAPI WebSocket using the same auth/dial behavior as the legacy CLI.
func DialWS(c *Client, path string) (*websocket.Conn, error) {
	wsURL := "wss://localhost:54321" + path
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // #nosec G402 local daemon endpoint
		},
	}
	header := map[string][]string{"Authorization": {"Bearer " + strings.TrimSpace(c.Token())}}
	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func terminalSize() (uint32, uint32, error) {
	ws, err := os.Stdout.Stat()
	if err != nil {
		return 0, 0, err
	}
	if (ws.Mode() & os.ModeCharDevice) == 0 {
		return 0, 0, fmt.Errorf("stdout is not terminal")
	}
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0, 0, err
	}
	return uint32(cols), uint32(rows), nil
}
