package fieldagent

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// recreateHandlerContext cancels the previous session context and returns a fresh one.
func recreateHandlerContext(ctx *context.Context, cancel *context.CancelFunc) {
	if *cancel != nil {
		(*cancel)()
	}
	*ctx, *cancel = context.WithCancel(context.Background())
}

func closeWebSocketConn(connMu *sync.RWMutex, conn **websocket.Conn) {
	connMu.Lock()
	defer connMu.Unlock()
	if *conn != nil {
		_ = (*conn).Close()
		*conn = nil
	}
}

func stopSessionPingTicker(ticker **time.Ticker) {
	if *ticker != nil {
		(*ticker).Stop()
		*ticker = nil
	}
}

func drainExecOutputBuffer(h *ExecSessionWebSocketHandler) {
	for {
		select {
		case frame := <-h.outputBuffer:
			h.bufferedFrames.Add(-1)
			h.bufferedSize.Add(-int64(len(frame.data)))
		default:
			h.bufferedFrames.Store(0)
			h.bufferedSize.Store(0)
			return
		}
	}
}

func drainLogOutputBuffer(h *LogSessionWebSocketHandler) {
	for {
		select {
		case frame := <-h.outputBuffer:
			h.bufferedFrames.Add(-1)
			h.bufferedSize.Add(-int64(len(frame.data)))
		default:
			h.bufferedFrames.Store(0)
			h.bufferedSize.Store(0)
			return
		}
	}
}

func execSessionRunning(status any) bool {
	running, ok := status.(bool)
	return ok && running
}
