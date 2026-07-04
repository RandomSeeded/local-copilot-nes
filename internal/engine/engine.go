// Package engine adapts the vendored cursortab sweep provider to nes.Engine:
// it turns a nes.Snapshot into cursortab's CompletionInput, runs the provider's
// batch Complete against a local (OpenAI-compatible) model, and maps the result
// back to a nes.Completion.
package engine

import (
	"context"
	"strings"

	sourcectx "cursortab/ctx"
	"cursortab/provider/sweep"
	"cursortab/types"

	"local-copilot-nes/internal/nes"
)

// Config configures the sweep-backed engine.
type Config struct {
	URL            string  // model endpoint, e.g. http://127.0.0.1:8000
	CompletionPath string  // e.g. /v1/completions
	Model          string  // e.g. sweep-next-edit-1.5B
	MaxTokens      int     // tokens to generate
	ContextSize    int     // window-trim budget (tokens); 0 => falls back to MaxTokens
	Temperature    float64 // usually 0
	AnchorMaxRatio float64 // how far into the window an edit may anchor (Gap 2); 0 => provider default 0.25
}

// DefaultConfig points at a local llama-server serving the sweep next-edit model.
func DefaultConfig() Config {
	return Config{
		URL:            "http://127.0.0.1:8000",
		CompletionPath: "/v1/completions",
		Model:          "sweep-next-edit-1.5B",
		MaxTokens:      256,
		ContextSize:    8192,
		Temperature:    0,
		AnchorMaxRatio: 0.5, // relaxed from the 0.25 default so edits can land further from the cursor
	}
}

type sweepEngine struct {
	provider *sweep.Provider
}

// NewSweep builds a sweep-backed nes.Engine.
func NewSweep(cfg Config) nes.Engine {
	pc := &types.ProviderConfig{
		ProviderURL:         cfg.URL,
		CompletionPath:      cfg.CompletionPath,
		ProviderModel:       cfg.Model,
		ProviderTemperature: cfg.Temperature,
		ProviderMaxTokens:   cfg.MaxTokens,
		ProviderContextSize: cfg.ContextSize,
		AnchorMaxRatio:      cfg.AnchorMaxRatio,
		PrivacyMode:         true,
	}
	return &sweepEngine{provider: sweep.NewProvider(pc)}
}

// Complete implements nes.Engine.
func (e *sweepEngine) Complete(ctx context.Context, snap nes.Snapshot) (*nes.Completion, error) {
	input := sourcectx.CompletionInput{
		Current: sourcectx.CurrentSnapshot{
			File: sourcectx.FileSnapshot{
				Path:  uriToPath(snap.URI),
				Lines: splitLines(snap.Text),
			},
			// cursortab: Row is 1-indexed, Col is 0-indexed bytes.
			Cursor: sourcectx.CursorPosition{Row: snap.Cursor.Line + 1, Col: snap.Cursor.Character},
		},
		Materials: buildMaterials(snap),
	}

	resp, err := e.provider.Complete(ctx, input)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Completion == nil {
		return nil, nil // no suggestion / no-op
	}

	c := resp.Completion
	// cursortab: StartLine/EndLineInc are 1-indexed inclusive.
	// nes.Completion: 0-indexed inclusive.
	return &nes.Completion{
		StartLine:  c.StartLine - 1,
		EndLineInc: c.EndLineInc - 1,
		Lines:      c.Lines,
	}, nil
}

// buildMaterials converts the snapshot's recent edits into the EditHistory
// material that feeds sweep's recent_changes prompt section (drives chaining).
func buildMaterials(snap nes.Snapshot) sourcectx.Materials {
	if len(snap.Recent) == 0 {
		return sourcectx.Materials{}
	}
	entries := make([]*types.DiffEntry, 0, len(snap.Recent))
	for _, e := range snap.Recent {
		if e.Before == e.After {
			continue
		}
		entries = append(entries, &types.DiffEntry{
			Original: e.Before,
			Updated:  e.After,
			Source:   types.DiffSourceManual,
		})
	}
	if len(entries) == 0 {
		return sourcectx.Materials{}
	}
	return sourcectx.Materials{
		sourcectx.EditHistory{
			Files: []*types.FileDiffHistory{{
				FileName:    uriToPath(snap.URI),
				DiffHistory: entries,
			}},
		},
	}
}

func splitLines(text string) []string {
	if text == "" {
		return []string{}
	}
	lines := strings.Split(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1] // drop the empty element after a trailing newline
	}
	return lines
}

func uriToPath(uri string) string {
	return strings.TrimPrefix(uri, "file://")
}
