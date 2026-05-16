# Third-Party Notices — telegram package

## tucnak/telebot v4

**Repository:** https://github.com/tucnak/telebot  
**License:** MIT  
**License file:** telegram/LICENSE.telebot.md  
**Files adapted:** `layout/layout.go`, `layout/parser.go` (selective)  
**Used in:** `telegram/locale.go` — YAML→struct loading + per-lang fallback logic  
**Modifications:**
- Removed `tele.Context` coupling and all bot-specific transport types
- Removed markup template DSL and inline-result types
- Retained: YAML schema, locale-keyed map structure, per-lang fallback pattern
- ~200 LOC kept from ~571 LOC source

**GPL-licensed patterns:** `gotgbot` patterns were reviewed as reference only —
zero code copied (GPL blocker per architecture spec §6).
