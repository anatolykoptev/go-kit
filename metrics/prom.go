package metrics

import "strings"

// parseLabeled разбирает имя метрики в синтаксисе kitmetrics.Label():
//
//	"rpc{service=auth,method=login}" → ("rpc", ["service","method"], ["auth","login"])
//	"wp_rest_calls"                   → ("wp_rest_calls", nil, nil).
//
// Невалидный синтаксис (без закрывающей скобки, пустые пары) возвращается как plain.
func parseLabeled(s string) (name string, keys, vals []string) {
	open := strings.IndexByte(s, '{')
	if open < 0 {
		return s, nil, nil
	}
	if !strings.HasSuffix(s, "}") {
		return s, nil, nil
	}
	name = s[:open]
	inner := s[open+1 : len(s)-1]
	if inner == "" {
		return s, nil, nil
	}
	for _, kv := range strings.Split(inner, ",") {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			return s, nil, nil // malformed
		}
		keys = append(keys, kv[:eq])
		vals = append(vals, kv[eq+1:])
	}
	return name, keys, vals
}
