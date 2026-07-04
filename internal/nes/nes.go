// Package nes implements the core of local-copilot-nes: it answers a client's
// next-edit-suggestion request by building a Snapshot of the focused document
// and asking an Engine for a candidate edit, then shaping it into the versioned
// LSP edit the client expects. It owns nothing about suggestion lifetime or
// rendering — that is the client's job.
package nes

import (
	"context"
	"strings"
	"sync"
)

// Position is a zero-based line/character coordinate.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a half-open [Start, End) span in a document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// TextDocumentID names a document and the version an edit is anchored to.
type TextDocumentID struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// Snapshot is the immutable input the Engine computes against.
type Snapshot struct {
	URI     string
	Text    string
	Version int
	Cursor  Position
}

// Completion is a whole-line replacement produced by the Engine: replace
// document line indices [StartLine, EndLineInc] (zero-based, inclusive) with
// Lines.
type Completion struct {
	StartLine  int
	EndLineInc int
	Lines      []string
}

// Engine turns a Snapshot into at most one candidate edit.
type Engine interface {
	Complete(ctx context.Context, snap Snapshot) (*Completion, error)
}

// EngineFunc adapts a function to the Engine interface.
type EngineFunc func(ctx context.Context, snap Snapshot) (*Completion, error)

// Complete implements Engine.
func (f EngineFunc) Complete(ctx context.Context, snap Snapshot) (*Completion, error) {
	return f(ctx, snap)
}

// NesEdit is one suggestion returned to the client, shaped as sidekick's NES
// edit: a range to replace, the replacement text, and the document version the
// edit is anchored to.
type NesEdit struct {
	Range        Range          `json:"range"`
	Text         string         `json:"text"`
	TextDocument TextDocumentID `json:"textDocument"`
}

// InlineEditParams is the request for a suggestion at a cursor position.
type InlineEditParams struct {
	TextDocument TextDocumentID `json:"textDocument"`
	Position     Position       `json:"position"`
}

// InlineEditResult carries zero or more suggestions.
type InlineEditResult struct {
	Edits []NesEdit `json:"edits"`
}

// DocumentStore holds the client-synced text and version of open documents. It
// is safe for concurrent use: request handlers read it while didOpen/didChange
// notifications write it.
type DocumentStore struct {
	mu   sync.RWMutex
	docs map[string]document
}

type document struct {
	text    string
	version int
}

// NewDocumentStore returns an empty store.
func NewDocumentStore() *DocumentStore {
	return &DocumentStore{docs: make(map[string]document)}
}

// Open records the full text and version of a newly opened document.
func (s *DocumentStore) Open(uri, text string, version int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[uri] = document{text: text, version: version}
}

// Change replaces a document's full text and version (full-sync).
func (s *DocumentStore) Change(uri, text string, version int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[uri] = document{text: text, version: version}
}

// snapshot builds a Snapshot for uri at cursor, or ok=false if unknown.
func (s *DocumentStore) snapshot(uri string, cursor Position) (Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.docs[uri]
	if !ok {
		return Snapshot{}, false
	}
	return Snapshot{URI: uri, Text: d.text, Version: d.version, Cursor: cursor}, true
}

// Handler answers inline-edit requests against a store and an engine.
type Handler struct {
	store  *DocumentStore
	engine Engine
}

// NewHandler wires a store and engine into a Handler.
func NewHandler(store *DocumentStore, engine Engine) *Handler {
	return &Handler{store: store, engine: engine}
}

// InlineEdit answers a copilotInlineEdit request: build the snapshot, ask the
// engine, and shape the completion into a versioned edit. Returns no edits when
// the document is unknown or the engine declines.
func (h *Handler) InlineEdit(ctx context.Context, p InlineEditParams) (InlineEditResult, error) {
	none := InlineEditResult{Edits: []NesEdit{}}
	snap, ok := h.store.snapshot(p.TextDocument.URI, p.Position)
	if !ok {
		return none, nil
	}
	c, err := h.engine.Complete(ctx, snap)
	if err != nil {
		return none, err
	}
	if c == nil {
		return none, nil
	}
	edit := NesEdit{
		Range: Range{
			Start: Position{Line: c.StartLine, Character: 0},
			End:   Position{Line: c.EndLineInc + 1, Character: 0},
		},
		Text:         strings.Join(c.Lines, "\n") + "\n",
		TextDocument: TextDocumentID{URI: snap.URI, Version: snap.Version},
	}
	return InlineEditResult{Edits: []NesEdit{edit}}, nil
}
