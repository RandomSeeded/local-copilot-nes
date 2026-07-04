// Package nes implements the core of local-copilot-nes: it answers a client's
// next-edit-suggestion request by building a Snapshot of the focused document
// and asking an Engine for a candidate edit, then shaping it into the versioned
// LSP edit the client expects. It owns nothing about suggestion lifetime or
// rendering — that is the client's job.
package nes

import (
	"context"
	"strings"
)

// Position is a zero-based line/character coordinate.
type Position struct {
	Line      int
	Character int
}

// Range is a half-open [Start, End) span in a document.
type Range struct {
	Start Position
	End   Position
}

// TextDocumentID names a document and the version an edit is anchored to.
type TextDocumentID struct {
	URI     string
	Version int
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

// NesEdit is one suggestion returned to the client.
type NesEdit struct {
	Range        Range
	Text         string
	TextDocument TextDocumentID
}

// InlineEditParams is the request for a suggestion at a cursor position.
type InlineEditParams struct {
	TextDocument TextDocumentID
	Position     Position
}

// InlineEditResult carries zero or more suggestions.
type InlineEditResult struct {
	Edits []NesEdit
}

// DocumentStore holds the client-synced text and version of open documents.
type DocumentStore struct {
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
	s.docs[uri] = document{text: text, version: version}
}

// Change replaces a document's full text and version (full-sync).
func (s *DocumentStore) Change(uri, text string, version int) {
	s.docs[uri] = document{text: text, version: version}
}

// snapshot builds a Snapshot for uri at cursor, or ok=false if unknown.
func (s *DocumentStore) snapshot(uri string, cursor Position) (Snapshot, bool) {
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
	snap, ok := h.store.snapshot(p.TextDocument.URI, p.Position)
	if !ok {
		return InlineEditResult{}, nil
	}
	c, err := h.engine.Complete(ctx, snap)
	if err != nil {
		return InlineEditResult{}, err
	}
	if c == nil {
		return InlineEditResult{}, nil
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
