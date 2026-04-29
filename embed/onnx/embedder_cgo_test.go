//go:build cgo

package onnx

// Compile-time interface check: verify the cgo Embedder satisfies the
// parent embed.Embedder interface (already declared in embedder.go via
// `var _ parentembed.Embedder = (*Embedder)(nil)`). This file exists so
// the cgo build also runs `go vet`-like validation.
