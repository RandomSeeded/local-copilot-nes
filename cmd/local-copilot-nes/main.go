// Command local-copilot-nes is an LSP server that answers sidekick.nvim's
// Next-Edit-Suggestion requests (textDocument/copilotInlineEdit) from a local
// model. It speaks JSON-RPC over stdio.
package main

import (
	"bufio"
	"context"
	"os"

	"local-copilot-nes/internal/app"
	"local-copilot-nes/internal/nes"
)

func main() {
	// SKELETON engine: always suggests replacing the cursor's line with a marker.
	// It exists to validate the sidekick <-> server integration end-to-end; the
	// real cursortab-backed engine replaces it.
	engine := nes.EngineFunc(func(_ context.Context, snap nes.Snapshot) (*nes.Completion, error) {
		line := snap.Cursor.Line
		return &nes.Completion{
			StartLine:  line,
			EndLineInc: line,
			Lines:      []string{"# local-copilot-nes: skeleton edit"},
		}, nil
	})

	srv := app.New(engine)
	if err := srv.Serve(context.Background(), bufio.NewReader(os.Stdin), os.Stdout); err != nil {
		os.Exit(1)
	}
}
