package chrome

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// defaultSatoriURL is the local satori-render sidecar endpoint. Override via
// the SATORI_URL env var (no trailing slash).
const defaultSatoriURL = "http://127.0.0.1:8910"

// satoriRenderTimeout caps the full HTTP round-trip. Warm renders complete in
// ~75ms; the generous budget covers cold-start (~900ms) plus network + retry
// margin without inviting indefinite hangs.
const satoriRenderTimeout = 30 * time.Second

// satoriPNGMagic is the 8-byte PNG file signature used to validate sidecar output.
var satoriPNGMagic = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

// satoriRequest mirrors the sidecar's POST /render JSON contract.
type satoriRequest struct {
	HTML   string   `json:"html"`
	Width  int      `json:"width"`
	Height int      `json:"height"`
	Fonts  []string `json:"fonts,omitempty"`
}

// satoriErrorBody mirrors the sidecar's non-2xx JSON response shape.
type satoriErrorBody struct {
	Error  string `json:"error"`
	Detail string `json:"detail"`
}

// satoriURL returns the configured sidecar base URL, honoring SATORI_URL.
func satoriURL() string {
	if v := os.Getenv("SATORI_URL"); v != "" {
		return v
	}
	return defaultSatoriURL
}

// SatoriRender posts the given HTML body to the satori-render sidecar and
// returns PNG bytes. The endpoint URL is read from the SATORI_URL env var
// (default "http://127.0.0.1:8910"). On non-2xx responses the JSON error
// body is wrapped into a Go error; on connection failure the URL is
// included in the wrapped error.
func SatoriRender(ctx context.Context, html string, width, height int, fonts []string) ([]byte, error) {
	base := satoriURL()
	url := base + "/render"

	body, err := json.Marshal(satoriRequest{
		HTML:   html,
		Width:  width,
		Height: height,
		Fonts:  fonts,
	})
	if err != nil {
		return nil, fmt.Errorf("satori: marshal request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, satoriRenderTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("satori: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("satori sidecar unavailable at %s: %w", base, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("satori: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e satoriErrorBody
		if jerr := json.Unmarshal(respBody, &e); jerr == nil && (e.Error != "" || e.Detail != "") {
			return nil, fmt.Errorf("satori: %s: %s", e.Error, e.Detail)
		}
		return nil, fmt.Errorf("satori: http %d: %s", resp.StatusCode, string(respBody))
	}

	if len(respBody) < len(satoriPNGMagic) || !bytes.Equal(respBody[:len(satoriPNGMagic)], satoriPNGMagic) {
		return nil, fmt.Errorf("satori: invalid PNG response (got %d bytes, missing magic)", len(respBody))
	}
	return respBody, nil
}
