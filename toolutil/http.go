package toolutil

import (
	"encoding/json"
	"fmt"
)

// CheckHTTPStatus returns an error string if HTTP status >= 400, or empty on success.
// If the body is a JSON object with a "message" field, it is used for a readable error.
func CheckHTTPStatus(body []byte, status int) string {
	if status < 400 { //nolint:mnd
		return ""
	}
	if msg := jsonMessage(body); msg != "" {
		return fmt.Sprintf("HTTP %d: %s", status, msg)
	}
	const truncLen = 500
	return fmt.Sprintf("HTTP %d: %s", status, TruncateStr(string(body), truncLen))
}

// jsonMessage extracts "message" from a JSON object body (e.g. WordPress REST errors).
func jsonMessage(body []byte) string {
	var obj struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &obj) == nil && obj.Message != "" {
		return obj.Message
	}
	return ""
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
