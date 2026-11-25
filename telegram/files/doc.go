// Package files downloads Telegram files (voice notes, photos, documents) by
// file_id without ever exposing the bot token.
//
// A Bot API download link is https://api.telegram.org/file/bot<TOKEN>/<path>:
// the credential is part of the URL. Code that builds that link ends up with
// the token in a variable, and from there in logs, traces and error strings
// (net/http's *url.Error prints the whole URL). Never build Telegram file URLs
// yourself. Hand a [Downloader] the file_id and it does the getFile call, builds
// the URL internally, fetches it and returns the bytes; the caller never sees or
// holds the URL.
//
//	d := files.New(token, files.WithMaxSize(10<<20))
//	rc, info, err := d.Download(ctx, msg.Voice.FileID)
//	if err != nil { ... } // safe to log: carries no token and no URL
//	defer rc.Close()
//	resp, err := sttClient.TranscribeReader(ctx, rc, "voice.ogg")
//
// # Guarantees
//
//   - Every returned error, and the text of every error in its chain, is
//     scrubbed with [tgsafe]: a *url.Error keeps its type (Timeout, errors.Is
//     and errors.As still work) but carries bot<redacted>; any other error whose
//     message holds the token is replaced by a message-only error.
//   - [Info] holds the file_id, file_unique_id, size and MIME type, never the
//     file path (and so never a URL).
//   - [Downloader] prints no token through any fmt verb (it implements
//     fmt.Formatter, so %d, %x and every other verb are covered), String,
//     GoString or slog; the token sits behind an unexported pointer whose own
//     formatter redacts too.
//   - Text taken from a server response is scrubbed in full (raw, query-escaped,
//     percent-encoded in any hex case, and decoded from up to three layers of
//     percent-encoding) and only then cut to a short cap.
//   - Every error crossing the package boundary is sanitised: a chain whose
//     nodes all print clean is returned as is; if any node prints the token (a
//     raw *url.Error behind a wrapper with a clean message included) the
//     result is a fresh error with the scrubbed message that keeps errors.Is
//     and Timeout() but hands nothing to errors.As.
//   - The base URL must be https, or http on a loopback host, with no
//     credentials.
//   - The size is bounded twice: a file whose declared size (getFile file_size,
//     or Content-Length) exceeds [WithMaxSize] is refused before any byte is
//     read, and the stream itself is cut off with [ErrTooLarge] if it delivers
//     more than the limit anyway.
//   - Redirects are never followed, and the package logs nothing.
//
// # What this does not protect against
//
//   - HTTP/2 debug logging (GODEBUG=http2debug) sees request paths, which hold
//     the token; do not enable it in production.
//   - A caller-supplied transport, proxy hook or filesystem that logs requests,
//     or ignores cancellation, is outside the guarantees: [WithHTTPClient]
//     trusts what it is given.
//   - An untrusted self-hosted endpoint that echoes the token into file_id or
//     the MIME type. Point the Downloader only at api.telegram.org or your own
//     Bot API server. The package scrubs those fields anyway, best effort.
//   - The non-secret file path stays in redacted URL errors
//     (https://.../file/bot<redacted>/voice/file_7.oga).
//
// The package is stdlib plus [tgsafe]; it imports no Telegram SDK. With the
// OvyFlash SDK, tgapi5.NewFileDownloader builds a Downloader from a bot, copying
// its token, endpoint and HTTP client.
package files
