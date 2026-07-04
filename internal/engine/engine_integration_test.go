package engine

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"local-copilot-nes/internal/nes"
)

func serverUp(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// Real end-to-end against the local model: given a file with a repeated bug and
// a recent_changes entry showing one fix, the engine should propose the same
// fix at the next site. Skips when llama-server isn't running.
func TestSweepEngine_ProposesPropagatedEdit(t *testing.T) {
	if !serverUp("127.0.0.1:8000") {
		t.Skip("llama-server not running on 127.0.0.1:8000")
	}

	file := "def handle_alice():\n" +
		"    return greetings(\"Alice\")\n" +
		"\n" +
		"def handle_bob():\n" +
		"    return greet(\"Bob\")\n"

	snap := nes.Snapshot{
		URI:     "file:///handlers.py",
		Text:    file,
		Version: 2,
		Cursor:  nes.Position{Line: 4, Character: 4}, // on `    return greet("Bob")`
		Recent: []nes.Edit{{
			Before: `    return greet("Alice")`,
			After:  `    return greetings("Alice")`,
		}},
	}

	c, err := NewSweep(DefaultConfig()).Complete(context.Background(), snap)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if c == nil {
		t.Fatal("expected an edit, got none")
	}
	got := strings.Join(c.Lines, "\n")
	if !strings.Contains(got, `greetings("Bob")`) {
		t.Errorf("expected greet->greetings propagation on Bob; got:\n%s", got)
	}
}

// A cancelled context must abort the engine promptly (aborting the model call)
// rather than blocking — the server-side half of $/cancelRequest. Deterministic;
// does not require the model to be running.
func TestSweepEngine_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call

	snap := nes.Snapshot{
		URI:     "file:///a.py",
		Text:    "x = 1\ny = 2\n",
		Version: 1,
		Cursor:  nes.Position{Line: 1, Character: 0},
	}

	done := make(chan error, 1)
	go func() {
		_, err := NewSweep(DefaultConfig()).Complete(ctx, snap)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected an error for a cancelled context, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Complete did not honor context cancellation (blocked)")
	}
}
