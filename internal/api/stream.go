package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"

	"github.com/FomoCLI/FomoCLI/internal/jx"
)

// StreamEvent is one message from a WebSocket stream, or a connection notice.
type StreamEvent struct {
	Msg  jx.V
	Raw  []byte
	Info string // connection status text when Msg is null
	Err  error  // non-fatal error (we will reconnect)
}

// Stream connects to a WebSocket endpoint and delivers messages to fn until
// ctx is cancelled. It reconnects with exponential backoff. A close with
// policy violation (1008, e.g. wrong plan for /ws/trades) is fatal.
func (c *Client) Stream(ctx context.Context, path string, q url.Values, fn func(StreamEvent)) error {
	u := c.WSURL(path, q)
	backoff := time.Second
	for {
		err := c.streamOnce(ctx, u, fn)
		if ctx.Err() != nil {
			return nil
		}
		var ce websocket.CloseError
		if errors.As(err, &ce) && ce.Code == websocket.StatusPolicyViolation {
			return &APIError{Status: 403, Message: "stream refused: " + ce.Reason}
		}
		fn(StreamEvent{Err: err, Info: "reconnecting in " + backoff.String()})
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) streamOnce(ctx context.Context, u string, fn func(StreamEvent)) error {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	hdr := http.Header{}
	hdr.Set("user-agent", c.UserAgent)
	conn, _, err := websocket.Dial(dialCtx, u, &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4 << 20)
	fn(StreamEvent{Info: "connected"})
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		v, perr := jx.Parse(data)
		if perr != nil {
			continue
		}
		fn(StreamEvent{Msg: v, Raw: data})
	}
}
