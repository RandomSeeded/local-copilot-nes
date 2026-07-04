module local-copilot-nes

go 1.26.4

require cursortab v0.0.0-00010101000000-000000000000

require (
	github.com/neovim/go-client v1.2.1 // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
)

replace cursortab => ./third_party/cursortab-server
