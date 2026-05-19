package botuserstest

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/anatolykoptev/go-kit/telegram/botusers"
)

// MemStore is an in-memory reference implementation of botusers.Store.
// It is intentionally simple — no production use — but must satisfy the
// full Store contract to serve as the contract-test reference.
//
// MemStore is safe for concurrent use.
type MemStore struct {
	mu    sync.RWMutex
	users map[string]*botusers.User // key: botID+":"+strconv(tgID)
}

// NewMemStore returns a fresh, empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{users: make(map[string]*botusers.User)}
}

func key(botID string, tgID int64) string {
	return botID + ":" + strconv.FormatInt(tgID, 10)
}

func (m *MemStore) resolveBot(botID string) (string, error) {
	if botID == "" {
		return "", botusers.ErrBotIDRequired
	}
	return botID, nil
}

// UpsertFromInitData implements botusers.Store.
func (m *MemStore) UpsertFromInitData(ctx context.Context, botID string, user botusers.TelegramUser, obs botusers.Observation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	bid, err := m.resolveBot(botID)
	if err != nil {
		return err
	}
	at := obs.At
	if at.IsZero() {
		at = time.Now()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	k := key(bid, user.TgID)
	if existing, ok := m.users[k]; ok {
		// Update mutable fields; first_seen_at is immutable.
		updated := *existing
		// Preserve existing non-empty names; UpsertFromCommand passes empty strings.
		if user.Username != "" {
			updated.Username = user.Username
		}
		if user.FirstName != "" {
			updated.FirstName = user.FirstName
		}
		if user.LastName != "" {
			updated.LastName = user.LastName
		}
		updated.Lang = user.Lang
		updated.IsPremium = user.IsPremium
		updated.IsBot = user.IsBot
		updated.LastSeenAt = at
		updated.TotalObservations = existing.TotalObservations + 1
		if obs.Platform != "" {
			updated.Platform = obs.Platform
		}
		if obs.Country != "" {
			updated.Country = obs.Country
		}
		m.users[k] = &updated
	} else {
		m.users[k] = &botusers.User{
			BotID:             bid,
			TgID:              user.TgID,
			Username:          user.Username,
			FirstName:         user.FirstName,
			LastName:          user.LastName,
			Lang:              user.Lang,
			IsPremium:         user.IsPremium,
			IsBot:             user.IsBot,
			Country:           obs.Country,
			Platform:          obs.Platform,
			FirstSeenAt:       at,
			LastSeenAt:        at,
			TotalObservations: 1,

		}
	}
	return nil
}

// UpsertFromCommand implements botusers.Store.
func (m *MemStore) UpsertFromCommand(ctx context.Context, botID string, chatID int64, lang string, obs botusers.Observation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	user := botusers.TelegramUser{TgID: chatID, Lang: lang}
	return m.UpsertFromInitData(ctx, botID, user, obs)
}

// Get implements botusers.Store.
func (m *MemStore) Get(ctx context.Context, botID string, tgID int64) (*botusers.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if botID == "" {
		return nil, botusers.ErrBotIDRequired
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	u, ok := m.users[key(botID, tgID)]
	if !ok {
		return nil, botusers.ErrNotFound
	}
	// Return a copy to prevent mutation.
	cp := *u

	return &cp, nil
}

// List implements botusers.Store with keyset pagination.
func (m *MemStore) List(ctx context.Context, filter botusers.Filter) ([]*botusers.User, botusers.Cursor, error) {
	if err := ctx.Err(); err != nil {
		return nil, botusers.Cursor{}, err
	}
	botID := filter.BotID
	if botID == "" {
		return nil, botusers.Cursor{}, botusers.ErrBotIDRequired
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	m.mu.RLock()
	var all []*botusers.User
	for _, u := range m.users {
		if u.BotID != botID {
			continue
		}
		cp := *u
		all = append(all, &cp)
	}
	m.mu.RUnlock()

	// Sort: last_seen_at DESC, tg_id ASC (stable pagination).
	sort.Slice(all, func(i, j int) bool {
		if !all[i].LastSeenAt.Equal(all[j].LastSeenAt) {
			return all[i].LastSeenAt.After(all[j].LastSeenAt)
		}
		return all[i].TgID < all[j].TgID
	})

	// Apply cursor filter.
	if filter.Cursor != nil && !filter.Cursor.IsZero() {
		curTs, curID, err := botusers.DecodeCursor(*filter.Cursor)
		if err != nil {
			return nil, botusers.Cursor{}, err
		}
		filtered := all[:0]
		for _, u := range all {
			after := u.LastSeenAt.Before(curTs) ||
				(u.LastSeenAt.Equal(curTs) && u.TgID > curID)
			if after {
				filtered = append(filtered, u)
			}
		}
		all = filtered
	}

	if len(all) == 0 {
		return nil, botusers.Cursor{}, nil
	}

	var page []*botusers.User
	if len(all) > limit {
		page = all[:limit]
	} else {
		page = all
	}

	var nextCursor botusers.Cursor
	if len(all) > limit {
		last := page[len(page)-1]
		nextCursor = botusers.EncodeCursor(last.LastSeenAt, last.TgID)
	}

	return page, nextCursor, nil
}

// Aggregate implements botusers.Store.
func (m *MemStore) Aggregate(ctx context.Context, botID string) (botusers.Aggregates, error) {
	if err := ctx.Err(); err != nil {
		return botusers.Aggregates{}, err
	}
	if botID == "" {
		return botusers.Aggregates{}, botusers.ErrBotIDRequired
	}

	now := time.Now()
	countryCounts := map[string]int64{}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var agg botusers.Aggregates
	for _, u := range m.users {
		if u.BotID != botID {
			continue
		}
		agg.Total++
		if u.IsPremium {
			agg.PremiumCount++
		}
		age := now.Sub(u.LastSeenAt)
		if age <= 24*time.Hour {
			agg.Active1D++
		}
		if age <= 7*24*time.Hour {
			agg.Active7D++
		}
		if age <= 30*24*time.Hour {
			agg.Active30D++
		}
		if u.Country != "" {
			countryCounts[u.Country]++
		}
	}

	// Build TopCountries sorted by count desc.
	type cc struct{ code string; cnt int64 }
	var ccs []cc
	for code, cnt := range countryCounts {
		ccs = append(ccs, cc{code, cnt})
	}
	sort.Slice(ccs, func(i, j int) bool { return ccs[i].cnt > ccs[j].cnt })
	for _, c := range ccs {
		agg.TopCountries = append(agg.TopCountries, botusers.CountryCount{Code: c.code, Count: c.cnt})
	}

	return agg, nil
}

// Forget implements botusers.Store.
func (m *MemStore) Forget(ctx context.Context, botID string, tgID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if botID == "" {
		return botusers.ErrBotIDRequired
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	k := key(botID, tgID)
	if _, ok := m.users[k]; !ok {
		return botusers.ErrNotFound
	}
	delete(m.users, k)
	return nil
}

// DeleteInactive implements botusers.Store.
func (m *MemStore) DeleteInactive(ctx context.Context, botID string, olderThan time.Duration) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if botID == "" {
		return 0, botusers.ErrBotIDRequired
	}
	cutoff := time.Now().Add(-olderThan)

	m.mu.Lock()
	defer m.mu.Unlock()

	var deleted int64
	for k, u := range m.users {
		if u.BotID == botID && u.LastSeenAt.Before(cutoff) {
			delete(m.users, k)
			deleted++
		}
	}
	return deleted, nil
}

// Ensure MemStore satisfies the Store interface at compile time.
var _ botusers.Store = (*MemStore)(nil)

