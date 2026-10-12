package ui

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Spinner is a tiny stderr spinner shown while a request is in flight. It is
// a no-op when stderr is not a terminal, so piped output stays clean.
type Spinner struct {
	msg  string
	stop chan struct{}
	wg   sync.WaitGroup
	on   bool
}

// Spin starts a spinner with a message.
func Spin(msg string) *Spinner {
	s := &Spinner{msg: msg, stop: make(chan struct{})}
	if !IsTTY(os.Stderr) || !ColorEnabled() {
		return s
	}
	s.on = true
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		t := time.NewTicker(80 * time.Millisecond)
		defer t.Stop()
		// Do not flash a spinner for fast (cached) responses.
		select {
		case <-s.stop:
			return
		case <-time.After(120 * time.Millisecond):
		}
		for i := 0; ; i++ {
			fmt.Fprintf(Err, "\r%s %s", Accent.Render(frames[i%len(frames)]), Muted.Render(s.msg))
			select {
			case <-s.stop:
				fmt.Fprint(Err, "\r\x1b[2K")
				return
			case <-t.C:
			}
		}
	}()
	return s
}

// Stop halts the spinner and clears its line.
func (s *Spinner) Stop() {
	if s == nil || !s.on {
		return
	}
	s.on = false
	close(s.stop)
	s.wg.Wait()
}
