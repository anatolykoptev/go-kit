package eventbus

import "testing"

func TestMatchTopic(t *testing.T) {
	cases := []struct {
		pattern, topic string
		want           bool
	}{
		{"a.b", "a.b", true},
		{"a.*", "a.b", true},
		{"a.*", "a.b.c", false},
		{"a.**", "a.b", true},
		{"a.**", "a.b.c.d", true},
		{"**", "anything", true},
		{"**", "a.b.c", true},
		{"*", "single", true},
		{"*", "a.b", false},
		{"a.*.c", "a.b.c", true},
		{"a.*.c", "a.b.d", false},
		{"a.b", "a.c", false},
	}
	for _, c := range cases {
		if got := matchTopic(c.pattern, c.topic); got != c.want {
			t.Errorf("matchTopic(%q, %q) = %v, want %v", c.pattern, c.topic, got, c.want)
		}
	}
}
