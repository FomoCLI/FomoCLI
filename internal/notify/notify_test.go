package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type hit struct {
	path, ctype string
	body        []byte
}

func capture(t *testing.T, status int) (*httptest.Server, func() []hit) {
	var mu sync.Mutex
	var hits []hit
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		hits = append(hits, hit{r.URL.Path, r.Header.Get("content-type"), b})
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []hit { mu.Lock(); defer mu.Unlock(); return append([]hit(nil), hits...) }
}

func TestTelegramAndDiscord(t *testing.T) {
	srv, hits := capture(t, 200)
	tg := &Telegram{Token: "123:abc", ChatID: "-100", API: srv.URL}
	dc := &Discord{WebhookURL: srv.URL + "/api/webhooks/1/xyz"}
	ctx := context.Background()
	if err := tg.Send(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := dc.Send(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := tg.SendPhoto(ctx, "cap", []byte("\x89PNG")); err != nil {
		t.Fatal(err)
	}
	if err := dc.SendPhoto(ctx, "cap", []byte("\x89PNG")); err != nil {
		t.Fatal(err)
	}
	h := hits()
	if len(h) != 4 {
		t.Fatalf("hits = %d", len(h))
	}
	if h[0].path != "/bot123:abc/sendMessage" {
		t.Errorf("telegram path %q", h[0].path)
	}
	var m map[string]any
	_ = json.Unmarshal(h[0].body, &m)
	if m["chat_id"] != "-100" || m["text"] != "hello" {
		t.Errorf("telegram body %s", h[0].body)
	}
	_ = json.Unmarshal(h[1].body, &m)
	if m["content"] != "hello" {
		t.Errorf("discord body %s", h[1].body)
	}
	if h[2].path != "/bot123:abc/sendPhoto" || !strings.HasPrefix(h[2].ctype, "multipart/form-data") {
		t.Errorf("telegram photo: %q %q", h[2].path, h[2].ctype)
	}
	if !strings.Contains(string(h[3].body), "payload_json") {
		t.Error("discord photo missing payload_json")
	}
}

func TestErrorsAndRedaction(t *testing.T) {
	srv, _ := capture(t, 403)
	err := (&Telegram{Token: "t", ChatID: "c", API: srv.URL}).Send(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("want 403 error, got %v", err)
	}
	err = (&Telegram{Token: "SECRET", ChatID: "c", API: "http://127.0.0.1:1"}).Send(context.Background(), "x")
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("token leaked in error: %v", err)
	}
	err = (&Discord{WebhookURL: "http://127.0.0.1:1/api/webhooks/1/TOPSECRET"}).Send(context.Background(), "x")
	if err == nil || strings.Contains(err.Error(), "TOPSECRET") {
		t.Fatalf("webhook leaked in error: %v", err)
	}
}

func TestDispatcher(t *testing.T) {
	srv, hits := capture(t, 200)
	d := &Dispatcher{Senders: []Sender{&Discord{WebhookURL: srv.URL}}, Interval: time.Millisecond}
	d.Start(context.Background())
	for i := 0; i < 3; i++ {
		if !d.Push("m") {
			t.Fatal("push dropped")
		}
	}
	d.Close(2 * time.Second)
	if n := len(hits()); n != 3 {
		t.Fatalf("delivered %d of 3", n)
	}
	var nilD *Dispatcher
	if nilD.Push("x") {
		t.Error("nil dispatcher should drop")
	}
}
