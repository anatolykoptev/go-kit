package telegram

import "testing"

// ---------------------------------------------------------------------------
// Detect
// ---------------------------------------------------------------------------

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Format
	}{
		// HTML branch
		{
			name: "bold tag",
			in:   "<b>hello</b>",
			want: FormatHTML,
		},
		{
			name: "strong tag",
			in:   "<strong>hello</strong>",
			want: FormatHTML,
		},
		{
			name: "em tag",
			in:   "text <em>italic</em> text",
			want: FormatHTML,
		},
		{
			name: "anchor tag",
			in:   `<a href="https://x.com">link</a>`,
			want: FormatHTML,
		},
		{
			name: "pre tag",
			in:   "<pre>code</pre>",
			want: FormatHTML,
		},
		{
			name: "heading tag h1",
			in:   "<h1>Title</h1>",
			want: FormatHTML,
		},
		{
			name: "div tag",
			in:   "<div>content</div>",
			want: FormatHTML,
		},
		{
			name: "tg-spoiler",
			in:   `<tg-spoiler>secret</tg-spoiler>`,
			want: FormatHTML,
		},
		// Markdown branch
		{
			name: "bold asterisks",
			in:   "**hello** world",
			want: FormatMarkdown,
		},
		{
			name: "bold underscores",
			in:   "__hello__ world",
			want: FormatMarkdown,
		},
		{
			name: "markdown link",
			in:   "[Google](https://google.com)",
			want: FormatMarkdown,
		},
		{
			name: "fenced code block",
			in:   "```go\nfmt.Println()\n```",
			want: FormatMarkdown,
		},
		{
			name: "heading hash",
			in:   "# Title\nsome content",
			want: FormatMarkdown,
		},
		{
			name: "list item dash",
			in:   "- item one\n- item two",
			want: FormatMarkdown,
		},
		// Plain branch
		{
			name: "plain text",
			in:   "just plain text without any markup",
			want: FormatPlain,
		},
		{
			name: "empty string",
			in:   "",
			want: FormatPlain,
		},
		{
			name: "plain with ampersand",
			in:   "foo & bar > baz",
			want: FormatPlain,
		},
		{
			name: "numeric text",
			in:   "12345 numbers only",
			want: FormatPlain,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Detect(tc.in)
			if got != tc.want {
				t.Errorf("Detect(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatString(t *testing.T) {
	tests := []struct {
		f    Format
		want string
	}{
		{FormatHTML, "HTML"},
		{FormatMarkdown, "Markdown"},
		{FormatPlain, "Plain"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := tc.f.String()
			if got != tc.want {
				t.Errorf("Format.String() = %q, want %q", got, tc.want)
			}
		})
	}
}
