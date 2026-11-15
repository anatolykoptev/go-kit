// Package voice synthesizes speech (text-to-speech) for chat bots and writes
// the result as an ogg/opus file, the format Telegram renders as a voice note.
//
// Providers:
//
//   - [EdgeProvider] drives the edge-tts CLI (free, no API key) and ffmpeg.
//   - [GeminiProvider] calls the Gemini Developer API and encodes the returned
//     WAV with ffmpeg.
//   - [FallbackProvider] runs a primary provider and falls back to a second one
//     when the primary is unavailable or fails.
//
// [NewProviderFromSettings] builds the usual composition from one [Settings]
// value. The library never reads environment variables: the Gemini API key is a
// constructor argument.
//
// Privacy: synthesized text is conversation content. No provider logs it, the
// text length is logged in runes, and neither CLI output nor HTTP error bodies
// are logged or returned (only exit status, output length, HTTP status and
// Google's error.status enum).
//
// Speech-to-text is not part of this package: use github.com/anatolykoptev/go-stt.
package voice
