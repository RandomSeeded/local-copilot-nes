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

// The engine may return a whole-window rewrite even when only a few lines
// change; we must tighten it to the changed hunk(s) so the client doesn't shade
// the whole range (the "whole file in a blue box" bug).
func TestInlineEdit_TightensWholeWindowRewriteToChangedHunk(t *testing.T) {
	s := NewDocumentStore()
	uri := "file:///a.py"
	s.Open(uri, "l0\nl1\nl2\nl3\nl4\n", 1)

	// engine "rewrites" the whole file but only changes l2 -> L2.
	engine := EngineFunc(func(_ context.Context, _ Snapshot) (*Completion, error) {
		return &Completion{StartLine: 0, EndLineInc: 4, Lines: []string{"l0", "l1", "L2", "l3", "l4"}}, nil
	})

	res, err := NewHandler(s, engine).InlineEdit(context.Background(), InlineEditParams{
		TextDocument: TextDocumentID{URI: uri, Version: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Edits) != 1 {
		t.Fatalf("want 1 tight edit, got %d: %+v", len(res.Edits), res.Edits)
	}
	e := res.Edits[0]
	if e.Range.Start.Line != 2 || e.Range.End.Line != 3 {
		t.Errorf("range not tightened to the changed line: got %+v want line 2->3", e.Range)
	}
	if e.Text != "L2\n" {
		t.Errorf("text: got %q want %q", e.Text, "L2\n")
	}
}

// Consecutive edits to the same line (e.g. typing a word one keystroke at a
// time) coalesce into a single meaningful recent edit, so recent_changes carries
// greet->greetings rather than greet->greeti->greetin->... fragments.
func TestChange_CoalescesConsecutiveEditsToSameLine(t *testing.T) {
	s := NewDocumentStore()
	uri := "file:///a.py"
	s.Open(uri, "x = greet\n", 1)
	for i, txt := range []string{"x = greeti\n", "x = greetin\n", "x = greeting\n", "x = greetings\n"} {
		s.Change(uri, txt, i+2)
	}

	snap, _ := s.snapshot(uri, Position{})
	if len(snap.Recent) != 1 {
		t.Fatalf("want 1 coalesced edit, got %d: %+v", len(snap.Recent), snap.Recent)
	}
	if e := snap.Recent[0]; e.Before != "x = greet" || e.After != "x = greetings" {
		t.Errorf("coalesced edit: got %+v want {greet -> greetings}", e)
	}
}

// Edits to different lines stay separate.
func TestChange_KeepsEditsToDifferentLinesSeparate(t *testing.T) {
	s := NewDocumentStore()
	uri := "file:///a.py"
	s.Open(uri, "a\nb\nc\n", 1)
	s.Change(uri, "A\nb\nc\n", 2) // line 0
	s.Change(uri, "A\nb\nC\n", 3) // line 2

	snap, _ := s.snapshot(uri, Position{})
	if len(snap.Recent) != 2 {
		t.Fatalf("want 2 separate edits, got %d: %+v", len(snap.Recent), snap.Recent)
	}
}

// Typing then deleting back to the original leaves no recent edit.
func TestChange_CoalesceToNoOpDropsEntry(t *testing.T) {
	s := NewDocumentStore()
	uri := "file:///a.py"
	s.Open(uri, "x = greet\n", 1)
	s.Change(uri, "x = greeti\n", 2)
	s.Change(uri, "x = greet\n", 3) // deleted back

	snap, _ := s.snapshot(uri, Position{})
	if len(snap.Recent) != 0 {
		t.Fatalf("want 0 edits after revert, got %d: %+v", len(snap.Recent), snap.Recent)
	}
}

// Reverting one edit must not corrupt an earlier edit at the same line index.
// Regression: lastLine was left stale after a revert-pop, so a later edit at
// that index coalesced into the wrong (earlier) recent entry.
func TestChange_RevertThenReeditSameLineDoesNotCorruptEarlierEdit(t *testing.T) {
	s := NewDocumentStore()
	uri := "file:///a.py"
	s.Open(uri, "l0\nl1\nl2\nl3\nl4\nl5\n", 1)
	s.Change(uri, "L0\nl1\nl2\nl3\nl4\nl5\n", 2) // edit line 0
	s.Change(uri, "L0\nl1\nl2\nl3\nl4\nL5\n", 3) // edit line 5
	s.Change(uri, "L0\nl1\nl2\nl3\nl4\nl5\n", 4) // revert line 5 (drops that entry)
	s.Change(uri, "L0\nl1\nl2\nl3\nl4\nX5\n", 5) // edit line 5 again

	snap, _ := s.snapshot(uri, Position{})
	// The line-0 edit must be intact, and the new line-5 edit must be its own entry.
	if len(snap.Recent) != 2 {
		t.Fatalf("want 2 edits, got %d: %+v", len(snap.Recent), snap.Recent)
	}
	if e := snap.Recent[0]; e.Before != "l0" || e.After != "L0" {
		t.Errorf("line-0 edit corrupted: got %+v want {l0 -> L0}", e)
	}
	if e := snap.Recent[1]; e.Before != "l5" || e.After != "X5" {
		t.Errorf("line-5 edit wrong: got %+v want {l5 -> X5}", e)
	}
}

// An edit whose changed span merely starts at the same line as the previous
// edit — but is not a continuation of it — must not coalesce (which produced an
// incoherent one-line-Before / multi-line-After entry).
func TestChange_DoesNotCoalesceMismatchedSpanAtSameLine(t *testing.T) {
	s := NewDocumentStore()
	uri := "file:///a.py"
	s.Open(uri, "l0\nl1\nl2\nl3\n", 1)
	s.Change(uri, "l0\nl1\nL2\nl3\n", 2)  // change only line 2
	s.Change(uri, "l0\nl1\nX2\nX3\n", 3)  // now change lines 2 AND 3

	snap, _ := s.snapshot(uri, Position{})
	for _, e := range snap.Recent {
		if e.Before == "l2" && e.After == "X2\nX3" {
			t.Fatalf("mismatched-span edits were coalesced into an incoherent entry: %+v", e)
		}
	}
}

// An edit through a final line that lacks a trailing newline must not append
// one (which would insert a spurious blank last line when the suggestion is
// applied).
func TestInlineEdit_NoSpuriousNewlineOnFinalLineWithoutTrailingNewline(t *testing.T) {
	s := NewDocumentStore()
	uri := "file:///a.py"
	s.Open(uri, "a\nb", 1) // no trailing newline

	engine := EngineFunc(func(_ context.Context, _ Snapshot) (*Completion, error) {
		return &Completion{StartLine: 0, EndLineInc: 1, Lines: []string{"a", "B"}}, nil
	})
	res, err := NewHandler(s, engine).InlineEdit(context.Background(), InlineEditParams{
		TextDocument: TextDocumentID{URI: uri, Version: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Edits) != 1 {
		t.Fatalf("want 1 edit, got %d: %+v", len(res.Edits), res.Edits)
	}
	if got := res.Edits[0].Text; got != "B" {
		t.Errorf("text: got %q want %q (no trailing newline appended at EOF)", got, "B")
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

// A didChange records the changed hunk as a recent edit, so the snapshot carries
// the recent_changes signal chaining depends on.
func TestChange_RecordsRecentEditForChaining(t *testing.T) {
	s := NewDocumentStore()
	uri := "file:///a.py"
	s.Open(uri, "a = 1\nx = greet(1)\nc = 3\n", 1)
	s.Change(uri, "a = 1\nx = greetings(1)\nc = 3\n", 2)

	snap, ok := s.snapshot(uri, Position{Line: 1})
	if !ok {
		t.Fatal("no snapshot")
	}
	if len(snap.Recent) != 1 {
		t.Fatalf("want 1 recent edit, got %d: %+v", len(snap.Recent), snap.Recent)
	}
	if e := snap.Recent[0]; e.Before != "x = greet(1)" || e.After != "x = greetings(1)" {
		t.Errorf("recent edit: got %+v", e)
	}
}
