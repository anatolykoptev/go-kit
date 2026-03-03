package toolutil

import "fmt"

// CheckHTTPStatus returns an error string if HTTP status >= 400, or empty on success.
func CheckHTTPStatus(body []byte, status int) string {
	if status < 400 { //nolint:mnd
		return ""
	}
	const truncLen = 500
	return fmt.Sprintf("HTTP %d: %s", status, TruncateStr(string(body), truncLen))
}

// SafeDate truncates an ISO date string to YYYY-MM-DD, or returns "unknown" for empty.
func SafeDate(d string) string {
	if len(d) >= 10 { //nolint:mnd
		return d[:10]
	}
	if d == "" {
		return "unknown"
	}
	return d
}
