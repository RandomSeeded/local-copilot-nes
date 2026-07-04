package nes

import (
	"context"
	"testing"
)

// The tracer bullet: a copilotInlineEdit request, against a document the client
// has opened and a fake Engine, produces one versioned LSP edit whose range,
// text, and echoed document version are correct.
func TestInlineEdit_ReturnsVersionedEditFromEngine(t *testing.T) {
	store := NewDocumentStore()
	uri := "file:///a.py"
	store.Open(uri, "x = 1\ny = 2\n", 1)

	engine := EngineFunc(func(_ context.Context, _ Snapshot) (*Completion, error) {
		// replace document line index 1 (0-based, inclusive) with "y = 3"
		return &Completion{StartLine: 1, EndLineInc: 1, Lines: []string{"y = 3"}}, nil
	})

	h := NewHandler(store, engine)

	res, err := h.InlineEdit(context.Background(), InlineEditParams{
		TextDocument: TextDocumentID{URI: uri, Version: 1},
		Position:     Position{Line: 1, Character: 0},
	})
	if err != nil {
		t.Fatalf("InlineEdit returned error: %v", err)
	}
	if len(res.Edits) != 1 {
		t.Fatalf("want 1 edit, got %d", len(res.Edits))
	}

	got := res.Edits[0]
	wantRange := Range{Start: Position{Line: 1, Character: 0}, End: Position{Line: 2, Character: 0}}
	if got.Range != wantRange {
		t.Errorf("range: got %+v want %+v", got.Range, wantRange)
	}
	if got.Text != "y = 3\n" {
		t.Errorf("text: got %q want %q", got.Text, "y = 3\n")
	}
	if got.TextDocument.URI != uri || got.TextDocument.Version != 1 {
		t.Errorf("versioned doc: got %+v want {uri:%s version:1}", got.TextDocument, uri)
	}
}

// After a didChange, requests compute against the new text and echo the new
// version — the version-tracking sidekick's persistence gate depends on.
func TestInlineEdit_UsesLatestVersionAfterChange(t *testing.T) {
	store := NewDocumentStore()
	uri := "file:///a.py"
	store.Open(uri, "x = 1\n", 1)
	store.Change(uri, "x = 1\ny = 2\n", 2)

	var seen Snapshot
	engine := EngineFunc(func(_ context.Context, snap Snapshot) (*Completion, error) {
		seen = snap
		return &Completion{StartLine: 0, EndLineInc: 0, Lines: []string{"x = 9"}}, nil
	})
	h := NewHandler(store, engine)

	res, err := h.InlineEdit(context.Background(), InlineEditParams{
		TextDocument: TextDocumentID{URI: uri, Version: 2},
		Position:     Position{Line: 1, Character: 0},
	})
	if err != nil {
		t.Fatalf("InlineEdit returned error: %v", err)
	}
	if seen.Text != "x = 1\ny = 2\n" {
		t.Errorf("snapshot text: got %q want updated text", seen.Text)
	}
	if seen.Version != 2 {
		t.Errorf("snapshot version: got %d want 2", seen.Version)
	}
	if len(res.Edits) != 1 || res.Edits[0].TextDocument.Version != 2 {
		t.Errorf("echoed version: got %+v want 2", res.Edits)
	}
}
