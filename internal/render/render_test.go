package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"
)

func TestInline(t *testing.T) {
	tests := map[string]struct {
		in   string
		want []Segment
	}{
		"plain": {in: "hello", want: []Segment{{Text: "hello"}}},
		"bold and code": {
			in:   "a **b** `c` d",
			want: []Segment{{Text: "a "}, {Text: "b", Bold: true}, {Text: " "}, {Text: "c", Code: true}, {Text: " d"}},
		},
		"markdown link": {
			in:   "see [repo#3](https://example.com/repo/issues/3) now",
			want: []Segment{{Text: "see "}, {Text: "repo#3", URL: "https://example.com/repo/issues/3"}, {Text: " now"}},
		},
		"bare url ends before Japanese text": {
			in:   "https://e.com/pull/1で確認",
			want: []Segment{{Text: "https://e.com/pull/1", URL: "https://e.com/pull/1"}, {Text: "で確認"}},
		},
		"bare url drops trailing punctuation": {
			in:   "(https://example.com/a?b=1)。",
			want: []Segment{{Text: "("}, {Text: "https://example.com/a?b=1", URL: "https://example.com/a?b=1"}, {Text: ")。"}},
		},
		"angle bracket autolink": {
			in:   "<https://example.com>",
			want: []Segment{{Text: "https://example.com", URL: "https://example.com"}},
		},
		"bold link": {
			in:   "**[x](https://e.com)**",
			want: []Segment{{Text: "x", Bold: true, URL: "https://e.com"}},
		},
		"unclosed markers stay text": {
			in:   "a ** b ` c [d](",
			want: []Segment{{Text: "a ** b ` c [d]("}},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, Inline(tt.in)); diff != "" {
				t.Errorf("Inline() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLinks(t *testing.T) {
	got := Links(Inline("[a](https://a.com) and https://b.com and [a2](https://a.com)"))
	want := []string{"https://a.com", "https://b.com"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Links() mismatch (-want +got):\n%s", diff)
	}
}

func plain(lines [][]Segment) []string {
	var out []string
	for _, l := range lines {
		var b strings.Builder
		for _, s := range l {
			b.WriteString(s.Text)
		}
		out = append(out, b.String())
	}
	return out
}

func TestWrap(t *testing.T) {
	tests := map[string]struct {
		in    string
		width int
		want  []string
	}{
		"fits":               {in: "abc def", width: 10, want: []string{"abc def"}},
		"breaks at spaces":   {in: "abc def ghi", width: 7, want: []string{"abc def", "ghi"}},
		"long word is cut":   {in: "abcdefghij", width: 4, want: []string{"abcd", "efgh", "ij"}},
		"wide chars":         {in: "日本語のテキスト", width: 6, want: []string{"日本語", "のテキ", "スト"}},
		"mixed keeps words":  {in: "これは test です", width: 8, want: []string{"これは", "test で", "す"}},
		"empty gives a line": {in: "", width: 5, want: []string{""}},
		"zero width no wrap": {in: "abc def", width: 0, want: []string{"abc def"}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := plain(Wrap(Inline(tt.in), tt.width))
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Wrap() mismatch (-want +got):\n%s", diff)
			}
			for _, l := range got {
				if tt.width > 0 && ansi.StringWidth(l) > tt.width {
					t.Errorf("line %q wider than %d", l, tt.width)
				}
			}
		})
	}
}

func TestWrapKeepsLinkOnEachLine(t *testing.T) {
	lines := Wrap(Inline("[aaaa bbbb](https://e.com)"), 5)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	for i, l := range lines {
		s := Encode(l)
		if !strings.Contains(s, "\x1b]8;;https://e.com\x1b\\") || !strings.HasSuffix(s, "\x1b]8;;\x1b\\") {
			t.Errorf("line %d = %q: want the link opened and closed on the line", i, s)
		}
	}
}

func TestTruncate(t *testing.T) {
	got := plain([][]Segment{Truncate(Inline("abc **def** ghi"), 6)})
	if diff := cmp.Diff([]string{"abc d…"}, got); diff != "" {
		t.Errorf("Truncate() mismatch (-want +got):\n%s", diff)
	}
	got = plain([][]Segment{Truncate(Inline("日本語"), 5)})
	if diff := cmp.Diff([]string{"日本…"}, got); diff != "" {
		t.Errorf("Truncate() wide mismatch (-want +got):\n%s", diff)
	}
	got = plain([][]Segment{Truncate(Inline("abc"), 3)})
	if diff := cmp.Diff([]string{"abc"}, got); diff != "" {
		t.Errorf("Truncate() fitting mismatch (-want +got):\n%s", diff)
	}
}

func TestEncodePlainHasNoEscapes(t *testing.T) {
	if got := Encode(Inline("plain text")); got != "plain text" {
		t.Errorf("Encode() = %q", got)
	}
}

func TestInlineOnlyLinksWebURLs(t *testing.T) {
	tests := map[string]struct {
		in   string
		want []Segment
	}{
		"relative path is text": {
			in:   "[design](design.md)",
			want: []Segment{{Text: "design"}},
		},
		"other scheme is text": {
			in:   "[x](file:///etc/passwd)",
			want: []Segment{{Text: "x"}},
		},
		"url with escape is not a link": {
			in:   "[x](https://e.com/\x1b]8;;evil)",
			want: []Segment{{Text: "x"}},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, Inline(tt.in)); diff != "" {
				t.Errorf("Inline() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEncodeDropsControlCharacters(t *testing.T) {
	got := Encode(Inline("a\x1b[2Jb\x07c"))
	if got != "a[2Jbc" {
		t.Errorf("Encode() = %q, want control characters dropped", got)
	}
}
