package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

const methodCancelRequest = "$/cancelRequest"

// Handler answers one JSON-RPC method. params is the raw request params (may be
// nil). A non-nil error becomes a JSON-RPC error response for requests.
type Handler func(ctx context.Context, params json.RawMessage) (any, error)

// Server routes JSON-RPC methods to handlers over a stdio transport.
type Server struct {
	handlers map[string]Handler
}

// NewServer returns an empty Server.
func NewServer() *Server {
	return &Server{handlers: make(map[string]Handler)}
}

// Handle registers h for method.
func (s *Server) Handle(method string, h Handler) {
	s.handlers[method] = h
}

type requestMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type responseMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// inflightEntry tracks a running request's cancel func plus a monotonic seq that
// identifies which registration owns the map slot for a given (possibly reused) id.
type inflightEntry struct {
	cancel context.CancelFunc
	seq    uint64
}

const (
	codeMethodNotFound = -32601
	codeInternalError  = -32603
)

// Serve reads framed messages from r until EOF. Requests are dispatched
// concurrently, each with a cancelable context so a later $/cancelRequest can
// abort an in-flight handler (e.g. a slow model call). Notifications are
// dispatched synchronously in read order, preserving document-sync ordering
// relative to the requests that follow them. Writes are serialized.
func (s *Server) Serve(ctx context.Context, r *bufio.Reader, w io.Writer) error {
	var writeMu sync.Mutex
	writeResp := func(resp responseMessage) {
		out, err := json.Marshal(resp)
		if err != nil {
			// Marshaling the payload failed; still send a well-formed error for
			// this id so the client doesn't hang forever waiting on a reply.
			out, err = json.Marshal(responseMessage{JSONRPC: "2.0", ID: resp.ID,
				Error: &rpcError{Code: codeInternalError, Message: "response marshal error: " + err.Error()}})
			if err != nil {
				return
			}
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = WriteMessage(w, out)
	}

	var mu sync.Mutex
	// inflight maps a request id to its cancel func. seq disambiguates a reused
	// id: a finishing request only deletes its own entry, never a newer request
	// that registered under the same id, so cancellation still reaches the new one.
	inflight := make(map[string]inflightEntry)
	var seq uint64
	var wg sync.WaitGroup

	for {
		body, err := ReadMessage(r)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			wg.Wait()
			return err
		}

		var req requestMessage
		if err := json.Unmarshal(body, &req); err != nil {
			continue // ignore unparseable input
		}

		if req.Method == methodCancelRequest {
			if key := cancelKey(req.Params); key != "" {
				mu.Lock()
				if e, ok := inflight[key]; ok {
					e.cancel()
				}
				mu.Unlock()
			}
			continue
		}

		h := s.handlers[req.Method]

		if req.ID == nil { // notification — run in order, no reply
			if h != nil {
				_, _ = h(ctx, req.Params)
			}
			continue
		}

		if h == nil {
			writeResp(responseMessage{JSONRPC: "2.0", ID: req.ID,
				Error: &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method}})
			continue
		}

		reqCtx, cancel := context.WithCancel(ctx)
		key := string(*req.ID)
		mu.Lock()
		seq++
		mySeq := seq
		inflight[key] = inflightEntry{cancel: cancel, seq: mySeq}
		mu.Unlock()

		wg.Add(1)
		go func(req requestMessage) {
			defer wg.Done()
			defer func() {
				mu.Lock()
				if e, ok := inflight[key]; ok && e.seq == mySeq {
					delete(inflight, key) // only remove our own entry, not a reused-id successor
				}
				mu.Unlock()
				cancel()
			}()

			resp := responseMessage{JSONRPC: "2.0", ID: req.ID}
			if result, herr := h(reqCtx, req.Params); herr != nil {
				resp.Error = &rpcError{Code: codeInternalError, Message: herr.Error()}
			} else if result == nil {
				// success with no payload must still send result: null (JSON-RPC),
				// else nvim rejects it as INVALID_SERVER_MESSAGE.
				resp.Result = json.RawMessage("null")
			} else {
				resp.Result = result
			}
			writeResp(resp)
		}(req)
	}

	wg.Wait()
	return nil
}

// cancelKey extracts the target request id from $/cancelRequest params as a
// string key matching how request ids are keyed in the inflight map.
func cancelKey(params json.RawMessage) string {
	var p struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return ""
	}
	return string(p.ID)
}
