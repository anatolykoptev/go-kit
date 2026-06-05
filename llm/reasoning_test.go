package llm

import "testing"

func TestSplitReasoning(t *testing.T) {
	cases := []struct{ name, content, rc, wantClean, wantReason string }{
		{"closed_inline", "<think>reasoning here</think>{\"ok\":true}", "", "{\"ok\":true}", "reasoning here"},
		{"live_minimax", "<think>The user asks for a city.</think>{\"city\":\"Paris\"}", "", "{\"city\":\"Paris\"}", "The user asks for a city."},
		{"unclosed", "<think>cut off mid thought", "", "", "cut off mid thought"},
		{"no_think", "{\"ok\":true}", "", "{\"ok\":true}", ""},
		{"rc_field_only", "{\"ok\":true}", "field reasoning", "{\"ok\":true}", "field reasoning"},
		{"both_sources", "<think>inline r</think>{\"x\":1}", "field r", "{\"x\":1}", "field r\ninline r"},
		{"leading_ws", "  <think>r</think>answer", "", "answer", "r"},
		{"think_not_leading", "answer text <think>not reasoning</think>", "", "answer text <think>not reasoning</think>", ""},
		{"empty", "", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clean, reason := splitReasoning(tc.content, tc.rc)
			if clean != tc.wantClean {
				t.Errorf("clean = %q, want %q", clean, tc.wantClean)
			}
			if reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}
