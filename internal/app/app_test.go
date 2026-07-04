package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"local-copilot-nes/internal/lsp"
	"local-copilot-nes/internal/nes"
)

// End-to-end over the transport: an initialize + didOpen + copilotInlineEdit
// session produces a well-formed versioned edit derived from the engine.
func TestServer_InlineEditOverLSP(t *testing.T) {
	engine := nes.EngineFunc(func(_ context.Context, snap nes.Snapshot) (*nes.Completion, error) {
		if snap.Text != "x = 1\ny = 2\n" {
			t.Errorf("engine saw text %q", snap.Text)
		}
		return &nes.Completion{StartLine: 1, EndLineInc: 1, Lines: []string{"y = 3"}}, nil
	})
	srv := New(engine)

	var in bytes.Buffer
	for _, m := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///a.py","version":1,"text":"x = 1\ny = 2\n"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"textDocument/copilotInlineEdit","params":{"textDocument":{"uri":"file:///a.py","version":1},"position":{"line":1,"character":0}}}`,
	} {
		if err := lsp.WriteMessage(&in, []byte(m)); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	if err := srv.Serve(context.Background(), bufio.NewReader(&in), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	// Two responses: initialize (id 1) and copilotInlineEdit (id 2).
	r := bufio.NewReader(bytes.NewReader(out.Bytes()))
	edit := readInlineEditResult(t, r) // reads until it finds the id-2 response
	if edit.Text != "y = 3\n" {
		t.Errorf("edit text: got %q want %q", edit.Text, "y = 3\n")
	}
	if edit.Range.Start.Line != 1 || edit.Range.End.Line != 2 {
		t.Errorf("edit range: got %+v", edit.Range)
	}
	if edit.TextDocument.URI != "file:///a.py" || edit.TextDocument.Version != 1 {
		t.Errorf("edit versioned-doc: got %+v", edit.TextDocument)
	}
}

func readInlineEditResult(t *testing.T, r *bufio.Reader) nes.NesEdit {
	t.Helper()
	for {
		body, err := lsp.ReadMessage(r)
		if err != nil {
			t.Fatalf("did not find copilotInlineEdit response: %v", err)
		}
		var resp struct {
			ID     int `json:"id"`
			Result *struct {
				Edits []nes.NesEdit `json:"edits"`
			} `json:"result"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.ID == 2 {
			if resp.Result == nil || len(resp.Result.Edits) != 1 {
				t.Fatalf("id-2 response has no single edit: %s", body)
			}
			return resp.Result.Edits[0]
		}
	}
}
