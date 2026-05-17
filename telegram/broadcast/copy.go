package broadcast

import (
	"context"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// batchChunkSize is the maximum number of message IDs per copyMessages or
// deleteMessages API call. Telegram Bot API cap: 100 messages per call.
const batchChunkSize = 100

// ChattableSender is the interface satisfied by *tgapi5.BotSender.
// It dispatches an arbitrary tgbotapi.Chattable and returns the sent message.
type ChattableSender interface {
	SendChattable(c tgbotapi.Chattable) (tgbotapi.Message, error)
}

// BatchCopyOptions configures a batched copy broadcast.
type BatchCopyOptions struct {
	FromChatID          int64
	MessageIDs          []int   // up to 100 per API call — chunked automatically
	ToChatIDs           []int64 // destination chats
	DisableNotification bool
	ProtectContent      bool
	RemoveCaption       bool
}

// TargetResult holds the per-target outcome of a broadcast operation.
type TargetResult struct {
	ChatID int64
	Err    error
}

// Broadcaster dispatches batched copy and delete operations over a
// ChattableSender while preserving the per-call rate enforced by the sender.
type Broadcaster struct {
	sender ChattableSender
}

// NewBroadcaster returns a Broadcaster backed by sender.
func NewBroadcaster(sender ChattableSender) *Broadcaster {
	return &Broadcaster{sender: sender}
}

// Copy sends opts.MessageIDs to every chat in opts.ToChatIDs using copyMessages.
// MessageIDs are automatically chunked at batchChunkSize (100) per API call.
// Returns one TargetResult per destination chat. The first error encountered
// for a target is recorded in TargetResult.Err; subsequent chunks for the same
// target are still attempted.
func (b *Broadcaster) Copy(ctx context.Context, opts BatchCopyOptions) []TargetResult {
	results := make([]TargetResult, 0, len(opts.ToChatIDs))
	chunks := chunkInts(opts.MessageIDs, batchChunkSize)

	for _, chatID := range opts.ToChatIDs {
		if ctx.Err() != nil {
			results = append(results, TargetResult{ChatID: chatID, Err: ctx.Err()})
			continue
		}
		var firstErr error
		for _, chunk := range chunks {
			if ctx.Err() != nil {
				if firstErr == nil {
					firstErr = ctx.Err()
				}
				break
			}
			cfg := tgbotapi.CopyMessagesConfig{
				BaseChat: tgbotapi.BaseChat{
					ChatConfig:          tgbotapi.ChatConfig{ChatID: chatID},
					DisableNotification: opts.DisableNotification,
					ProtectContent:      opts.ProtectContent,
				},
				FromChat:      tgbotapi.ChatConfig{ChatID: opts.FromChatID},
				MessageIDs:    chunk,
				RemoveCaption: opts.RemoveCaption,
			}
			if _, err := b.sender.SendChattable(cfg); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		results = append(results, TargetResult{ChatID: chatID, Err: firstErr})
	}
	return results
}

// chunkInts splits s into sub-slices of at most size elements.
// Returns an empty (non-nil) slice of chunks when s is empty.
func chunkInts(s []int, size int) [][]int {
	if len(s) == 0 {
		return [][]int{}
	}
	chunks := make([][]int, 0, (len(s)+size-1)/size)
	for len(s) > 0 {
		n := size
		if n > len(s) {
			n = len(s)
		}
		chunks = append(chunks, s[:n])
		s = s[n:]
	}
	return chunks
}
