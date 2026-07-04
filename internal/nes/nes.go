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
	Recent  []Edit // recent changes to this document, newest last
}

// Edit is a before/after text pair describing a recent change — supplied to the
// engine so it can propose the analogous next edit (drives chaining).
type Edit struct {
	Before string
	After  string
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
	text     string
	version  int
	recent   []Edit
	lastLine int // first-changed line of the most recent edit, for coalescing
}

// maxRecent bounds how many recent edits we keep per document for chaining.
const maxRecent = 10

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

// Change replaces a document's full text and version (full-sync), recording the
// changed hunk as a recent edit for chaining. Consecutive edits to the same
// starting line coalesce into one (keeping the original "before"), so per-
// keystroke typing becomes a single meaningful before/after rather than a run
// of fragments.
func (s *DocumentStore) Change(uri, text string, version int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.docs[uri]
	if line, edit, ok := diffEditAtLine(d.text, text); ok {
		if len(d.recent) > 0 && line == d.lastLine {
			last := &d.recent[len(d.recent)-1]
			last.After = edit.After
			if last.Before == last.After {
				d.recent = d.recent[:len(d.recent)-1] // typed then reverted
			}
		} else {
			d.recent = append(d.recent, edit)
			if len(d.recent) > maxRecent {
				d.recent = d.recent[len(d.recent)-maxRecent:]
			}
		}
		d.lastLine = line
	}
	d.text = text
	d.version = version
	s.docs[uri] = d
}

// snapshot builds a Snapshot for uri at cursor, or ok=false if unknown.
func (s *DocumentStore) snapshot(uri string, cursor Position) (Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.docs[uri]
	if !ok {
		return Snapshot{}, false
	}
	return Snapshot{
		URI:     uri,
		Text:    d.text,
		Version: d.version,
		Cursor:  cursor,
		Recent:  append([]Edit(nil), d.recent...),
	}, true
}

// diffEditAtLine reduces a full-text change to the minimal changed line hunk as
// a before/after pair, returning the first changed line index (used to coalesce
// consecutive edits). Returns ok=false when there is no prior text or no change.
func diffEditAtLine(oldText, newText string) (line int, e Edit, ok bool) {
	if oldText == "" || oldText == newText {
		return 0, Edit{}, false
	}
	o := strings.Split(oldText, "\n")
	n := strings.Split(newText, "\n")

	p := 0
	for p < len(o) && p < len(n) && o[p] == n[p] {
		p++
	}
	so, sn := len(o), len(n)
	for so > p && sn > p && o[so-1] == n[sn-1] {
		so--
		sn--
	}
	before := strings.Join(o[p:so], "\n")
	after := strings.Join(n[p:sn], "\n")
	if before == after {
		return 0, Edit{}, false
	}
	return p, Edit{Before: before, After: after}, true
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

	// The engine returns a whole-window replacement, but often only a few lines
	// differ. Diff the replaced region against the replacement and emit one tight
	// edit per changed hunk, so the client highlights only what changes rather
	// than shading the whole window.
	doc := splitDocLines(snap.Text)
	start := clamp(c.StartLine, 0, len(doc))
	end := clamp(c.EndLineInc+1, start, len(doc))
	region := doc[start:end]

	edits := make([]NesEdit, 0)
	for _, hk := range lineHunks(region, c.Lines) {
		at := start + hk.oldStart
		text := ""
		if len(hk.newLines) > 0 {
			text = strings.Join(hk.newLines, "\n") + "\n"
		}
		edits = append(edits, NesEdit{
			Range: Range{
				Start: Position{Line: at, Character: 0},
				End:   Position{Line: at + hk.oldCount, Character: 0},
			},
			Text:         text,
			TextDocument: TextDocumentID{URI: snap.URI, Version: snap.Version},
		})
	}
	if len(edits) == 0 {
		return none, nil
	}
	return InlineEditResult{Edits: edits}, nil
}

// lineHunk is a contiguous line replacement: replace oldCount lines starting at
// oldStart (indices into the replaced region) with newLines.
type lineHunk struct {
	oldStart int
	oldCount int
	newLines []string
}

// lineHunks computes minimal contiguous line-level changes between a and b via
// an LCS, so unchanged lines between changes split into separate hunks.
func lineHunks(a, b []string) []lineHunk {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var hs []lineHunk
	i, j := 0, 0
	for i < n || j < m {
		if i < n && j < m && a[i] == b[j] {
			i++
			j++
			continue
		}
		oldStart := i
		var repl []string
		for i < n || j < m {
			if i < n && j < m && a[i] == b[j] {
				break
			}
			if j < m && (i >= n || lcs[i][j+1] >= lcs[i+1][j]) {
				repl = append(repl, b[j])
				j++
			} else {
				i++
			}
		}
		hs = append(hs, lineHunk{oldStart: oldStart, oldCount: i - oldStart, newLines: repl})
	}
	return hs
}

func splitDocLines(text string) []string {
	if text == "" {
		return []string{}
	}
	lines := strings.Split(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
