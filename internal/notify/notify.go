// Package notify pushes alerts from fomocli to chat apps: a Telegram bot or a
// Discord webhook. Messages are queued and sent one at a time so a burst of
// trades never blocks the terminal or trips the chat app's rate limits.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Sender delivers one text message.
type Sender interface {
	Name() string
	Send(ctx context.Context, text string) error
}

// Telegram sends through a bot (create one with @BotFather).
type Telegram struct {
	Token  string
	ChatID string
	API    string // override for tests; default https://api.telegram.org
	HTTP   *http.Client
}

func (t *Telegram) Name() string { return "telegram" }

func (t *Telegram) Send(ctx context.Context, text string) error {
	api := t.API
	if api == "" {
		api = "https://api.telegram.org"
	}
	body := map[string]any{"chat_id": t.ChatID, "text": text, "disable_web_page_preview": true}
	return postJSON(ctx, t.HTTP, api+"/bot"+t.Token+"/sendMessage", body)
}

// Discord posts to a channel webhook (Channel settings → Integrations → Webhooks).
type Discord struct {
	WebhookURL string
	HTTP       *http.Client
}

func (d *Discord) Name() string { return "discord" }

func (d *Discord) Send(ctx context.Context, text string) error {
	if len(text) > 1900 {
		text = text[:1900] + "…"
	}
	return postJSON(ctx, d.HTTP, d.WebhookURL, map[string]any{"content": text, "username": "FomoCLI"})
}

func postJSON(ctx context.Context, c *http.Client, url string, body any) error {
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	res, err := c.Do(req)
	if err != nil {
		return redact(err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// redact keeps bot tokens and webhook secrets out of error messages.
func redact(err error) error {
	s := err.Error()
	if i := strings.Index(s, "/bot"); i >= 0 {
		if j := strings.Index(s[i:], "/sendMessage"); j > 0 {
			s = s[:i] + "/bot<token>" + s[i+j:]
		}
	}
	if i := strings.Index(s, "/api/webhooks/"); i >= 0 {
		end := strings.IndexAny(s[i:], "\": ")
		if end < 0 {
			end = len(s) - i
		}
		s = s[:i] + "/api/webhooks/<secret>" + s[i+end:]
	}
	return fmt.Errorf("%s", s)
}

// Dispatcher queues messages and delivers them to every sender in the
// background. Failures are reported through OnError, never fatal.
type Dispatcher struct {
	Senders  []Sender
	OnError  func(sender string, err error)
	Interval time.Duration // gap between messages (default 1s)

	once  sync.Once
	queue chan string
	wg    sync.WaitGroup
}

// Start launches the delivery loop. It stops when ctx is done or Close is called.
func (d *Dispatcher) Start(ctx context.Context) {
	d.once.Do(func() {
		d.queue = make(chan string, 64)
		if d.Interval == 0 {
			d.Interval = time.Second
		}
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			for msg := range d.queue {
				for _, s := range d.Senders {
					sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					if err := s.Send(sctx, msg); err != nil && d.OnError != nil {
						d.OnError(s.Name(), err)
					}
					cancel()
				}
				select {
				case <-ctx.Done():
				case <-time.After(d.Interval):
				}
			}
		}()
	})
}

// Push queues a message; when the queue is full the message is dropped.
func (d *Dispatcher) Push(text string) bool {
	if d == nil || len(d.Senders) == 0 || d.queue == nil {
		return false
	}
	select {
	case d.queue <- text:
		return true
	default:
		return false
	}
}

// Close flushes queued messages (up to the timeout) and stops the loop.
func (d *Dispatcher) Close(timeout time.Duration) {
	if d == nil || d.queue == nil {
		return
	}
	close(d.queue)
	done := make(chan struct{})
	go func() { d.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// PhotoSender is implemented by senders that can post an image.
type PhotoSender interface {
	SendPhoto(ctx context.Context, caption string, png []byte) error
}

// SendPhoto posts a PNG with a caption via sendPhoto.
func (t *Telegram) SendPhoto(ctx context.Context, caption string, png []byte) error {
	api := t.API
	if api == "" {
		api = "https://api.telegram.org"
	}
	return postMultipart(ctx, t.HTTP, api+"/bot"+t.Token+"/sendPhoto",
		map[string]string{"chat_id": t.ChatID, "caption": caption}, "photo", png)
}

// SendPhoto uploads a PNG to the webhook channel.
func (d *Discord) SendPhoto(ctx context.Context, caption string, png []byte) error {
	payload, _ := json.Marshal(map[string]any{"content": caption, "username": "FomoCLI"})
	return postMultipart(ctx, d.HTTP, d.WebhookURL, map[string]string{"payload_json": string(payload)}, "files[0]", png)
}

func postMultipart(ctx context.Context, c *http.Client, url string, fields map[string]string, fileField string, png []byte) error {
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Second}
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, err := mw.CreateFormFile(fileField, "fomocli-card.png")
	if err != nil {
		return err
	}
	if _, err := fw.Write(png); err != nil {
		return err
	}
	mw.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("content-type", mw.FormDataContentType())
	res, err := c.Do(req)
	if err != nil {
		return redact(err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
