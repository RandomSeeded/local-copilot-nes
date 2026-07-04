// Package app wires the LSP method surface sidekick's NES client uses onto the
// nes handler + document store. It is the only place LSP JSON shapes are
// decoded; everything below speaks nes types.
package app

import (
	"context"
	"encoding/json"

	"local-copilot-nes/internal/lsp"
	"local-copilot-nes/internal/nes"
)

// New builds an lsp.Server backed by engine.
func New(engine nes.Engine) *lsp.Server {
	store := nes.NewDocumentStore()
	handler := nes.NewHandler(store, engine)
	s := lsp.NewServer()

	s.Handle("initialize", func(_ context.Context, _ json.RawMessage) (any, error) {
		return map[string]any{
			"capabilities": map[string]any{
				"textDocumentSync": 1, // full document sync
			},
			"serverInfo": map[string]any{"name": "local-copilot-nes"},
		}, nil
	})

	s.Handle("textDocument/didOpen", func(_ context.Context, p json.RawMessage) (any, error) {
		var params struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
				Text    string `json:"text"`
			} `json:"textDocument"`
		}
		if err := json.Unmarshal(p, &params); err != nil {
			return nil, err
		}
		store.Open(params.TextDocument.URI, params.TextDocument.Text, params.TextDocument.Version)
		return nil, nil
	})

	s.Handle("textDocument/didChange", func(_ context.Context, p json.RawMessage) (any, error) {
		var params struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if err := json.Unmarshal(p, &params); err != nil {
			return nil, err
		}
		if n := len(params.ContentChanges); n > 0 {
			// full-sync: the last change carries the whole document text
			store.Change(params.TextDocument.URI, params.ContentChanges[n-1].Text, params.TextDocument.Version)
		}
		return nil, nil
	})

	// didFocus is Copilot-specific ceremony; we have nothing to track.
	s.Handle("textDocument/didFocus", func(_ context.Context, _ json.RawMessage) (any, error) {
		return nil, nil
	})

	s.Handle("textDocument/copilotInlineEdit", func(ctx context.Context, p json.RawMessage) (any, error) {
		var params nes.InlineEditParams
		if err := json.Unmarshal(p, &params); err != nil {
			return nil, err
		}
		return handler.InlineEdit(ctx, params)
	})

	// Accept-telemetry hook — nothing to do for a local server.
	s.Handle("workspace/executeCommand", func(_ context.Context, _ json.RawMessage) (any, error) {
		return nil, nil
	})

	s.Handle("shutdown", func(_ context.Context, _ json.RawMessage) (any, error) {
		return nil, nil
	})

	return s
}
