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
//   - [Downloader] prints no token through %v, %+v, %#v, String, GoString or
//     slog (LogValue).
//   - The size is bounded twice: a file whose declared size (getFile file_size,
//     or Content-Length) exceeds [WithMaxSize] is refused before any byte is
//     read, and the stream itself is cut off with [ErrTooLarge] if it delivers
//     more than the limit anyway.
//   - Redirects are never followed, and the package logs nothing.
//
// The package is stdlib plus [tgsafe]; it imports no Telegram SDK. With the
// OvyFlash SDK, tgapi5.NewFileDownloader builds a Downloader from a bot.
package files
