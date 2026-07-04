package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"
)

func serve(t *testing.T, s *Server, requests ...string) []byte {
	t.Helper()
	var in bytes.Buffer
	for _, r := range requests {
		if err := WriteMessage(&in, []byte(r)); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := s.Serve(context.Background(), bufio.NewReader(&in), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	return out.Bytes()
}

func TestServe_DispatchesRequestToHandler(t *testing.T) {
	s := NewServer()
	s.Handle("ping", func(_ context.Context, _ json.RawMessage) (any, error) {
		return map[string]string{"pong": "ok"}, nil
	})

	out := serve(t, s, `{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`)

	body, err := ReadMessage(bufio.NewReader(bytes.NewReader(out)))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var resp struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, body)
	}
	if resp.JSONRPC != "2.0" || resp.ID != 1 {
		t.Errorf("envelope: got %+v", resp)
	}
	if string(resp.Result) != `{"pong":"ok"}` {
		t.Errorf("result: got %s", resp.Result)
	}
}

func TestServe_NotificationProducesNoReply(t *testing.T) {
	s := NewServer()
	var gotParams string
	s.Handle("noted", func(_ context.Context, p json.RawMessage) (any, error) {
		gotParams = string(p)
		return nil, nil
	})

	out := serve(t, s, `{"jsonrpc":"2.0","method":"noted","params":{"x":1}}`)

	if len(out) != 0 {
		t.Errorf("notification should produce no reply, got %q", out)
	}
	if gotParams != `{"x":1}` {
		t.Errorf("handler params: got %s", gotParams)
	}
}

func TestServe_UnknownRequestMethodReturnsMethodNotFound(t *testing.T) {
	out := serve(t, NewServer(), `{"jsonrpc":"2.0","id":7,"method":"nope"}`)

	body, err := ReadMessage(bufio.NewReader(bytes.NewReader(out)))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var resp struct {
		ID    int `json:"id"`
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != 7 || resp.Error == nil || resp.Error.Code != -32601 {
		t.Errorf("want id 7 + method-not-found (-32601), got %+v", resp)
	}
}

// A $/cancelRequest for an in-flight request must cancel that request's context.
// With synchronous dispatch the slow handler would block the read loop forever;
// this forces concurrent dispatch + per-id cancellation.
func TestServe_CancelRequestCancelsInFlightHandler(t *testing.T) {
	s := NewServer()
	s.Handle("slow", func(ctx context.Context, _ json.RawMessage) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	done := make(chan []byte, 1)
	go func() {
		var in bytes.Buffer
		_ = WriteMessage(&in, []byte(`{"jsonrpc":"2.0","id":1,"method":"slow"}`))
		_ = WriteMessage(&in, []byte(`{"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":1}}`))
		var out bytes.Buffer
		_ = s.Serve(context.Background(), bufio.NewReader(&in), &out)
		done <- out.Bytes()
	}()

	select {
	case out := <-done:
		body, err := ReadMessage(bufio.NewReader(bytes.NewReader(out)))
		if err != nil {
			t.Fatalf("expected an error response for the cancelled request: %v", err)
		}
		var resp struct {
			ID    int `json:"id"`
			Error *struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.ID != 1 || resp.Error == nil {
			t.Errorf("want id 1 with an error (cancelled), got %+v", resp)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return: $/cancelRequest did not cancel the in-flight handler")
	}
}

