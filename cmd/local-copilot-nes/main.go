// Command local-copilot-nes is an LSP server that answers sidekick.nvim's
// Next-Edit-Suggestion requests (textDocument/copilotInlineEdit) from a local
// model. It speaks JSON-RPC over stdio.
package main

import (
	"bufio"
	"context"
	"flag"
	"os"

	"local-copilot-nes/internal/app"
	"local-copilot-nes/internal/engine"
)

func main() {
	cfg := engine.DefaultConfig()
	flag.StringVar(&cfg.URL, "url", cfg.URL, "model endpoint (OpenAI-compatible)")
	flag.StringVar(&cfg.CompletionPath, "completion-path", cfg.CompletionPath, "completion path on the endpoint")
	flag.StringVar(&cfg.Model, "model", cfg.Model, "model name")
	flag.IntVar(&cfg.MaxTokens, "max-tokens", cfg.MaxTokens, "max tokens to generate")
	flag.IntVar(&cfg.ContextSize, "context-size", cfg.ContextSize, "window-trim token budget")
	flag.Parse()

	srv := app.New(engine.NewSweep(cfg))
	if err := srv.Serve(context.Background(), bufio.NewReader(os.Stdin), os.Stdout); err != nil {
		os.Exit(1)
	}
}
