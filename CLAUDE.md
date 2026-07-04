# CLAUDE.md

## Release

Версию присваивает **только release-please**. Смёржил `chore(main): release X` PR →
он сам тегает и создаёт GitHub Release.

**Ручной `git tag` / `gh release create` — запрещено.** Обгоняет
`.release-please-manifest.json` → рассинхрон. Инцидент: v0.95.0 затегали вручную,
манифест остался на 0.94.0, четыре репо уже тянули v0.95.0 как зависимость — чинили
PR #147 (resync манифеста, не снос тега).

Нужен номер версии раньше, чем PR готов? Смёржи release-please PR, не тегай сам.
