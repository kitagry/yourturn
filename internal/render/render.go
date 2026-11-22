// Package render turns one line of Markdown into styled terminal text: bold,
// inline code and links (as OSC 8 hyperlinks), wrapped or truncated to a width.
//
// Only inline syntax is handled; the line structure of ACTIONS.md is the
// actions package's job.
package render

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

type Segment struct {
	Text string
	Bold bool
	Code bool
	// URL makes the segment a link.
	URL string
}

var (
	mdLink   = regexp.MustCompile(`^\[([^\]]+)\]\(([^)\s]+)\)`)
	autoLink = regexp.MustCompile(`^<(https?://[^>\s]+)>`)
	// A bare URL ends at the first non-ASCII character: Japanese text often
	// follows a URL without a space ("https://e.com/pull/1で確認").
	bareURL = regexp.MustCompile(`^https?://[^\x00-\x20<>\]\x7f-\x{10FFFF}]+`)
)

// Inline splits md into segments. Unclosed markers are kept as text.
func Inline(md string) []Segment {
	return parse(md, Segment{})
}

func parse(md string, base Segment) []Segment {
	var out []Segment
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			s := base
			s.Text = stripControl(text.String())
			out = append(out, s)
			text.Reset()
		}
	}
	for i := 0; i < len(md); {
		rest := md[i:]
		switch {
		case strings.HasPrefix(rest, "**"):
			if end := strings.Index(rest[2:], "**"); end > 0 {
				flush()
				b := base
				b.Bold = true
				out = append(out, parse(rest[2:2+end], b)...)
				i += 2 + end + 2
				continue
			}
		case rest[0] == '`':
			if end := strings.IndexByte(rest[1:], '`'); end > 0 {
				flush()
				c := base
				c.Code, c.Text = true, stripControl(rest[1:1+end])
				out = append(out, c)
				i += 1 + end + 1
				continue
			}
		case rest[0] == '[':
			if m := mdLink.FindStringSubmatch(rest); m != nil {
				flush()
				l := base
				// Other targets (relative paths, file:) are shown as text: the
				// viewer cannot resolve them and should not hand them to open.
				if webURL(m[2]) {
					l.URL = m[2]
				}
				out = append(out, parse(m[1], l)...)
				i += len(m[0])
				continue
			}
		case rest[0] == '<':
			if m := autoLink.FindStringSubmatch(rest); m != nil && webURL(m[1]) {
				flush()
				l := base
				l.URL, l.Text = m[1], m[1]
				out = append(out, l)
				i += len(m[0])
				continue
			}
		case base.URL == "" && (strings.HasPrefix(rest, "http://") || strings.HasPrefix(rest, "https://")):
			if u := trimURL(bareURL.FindString(rest)); u != "" && webURL(u) {
				flush()
				l := base
				l.URL, l.Text = u, u
				out = append(out, l)
				i += len(u)
				continue
			}
		}
		text.WriteByte(md[i])
		i++
	}
	flush()
	return out
}

// webURL is whether u is safe to make a link of: http(s) only, and no control
// characters, which would end the OSC 8 sequence and inject others.
func webURL(u string) bool {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return false
	}
	return !strings.ContainsFunc(u, isControl)
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0)
}

// stripControl drops control characters so text from the file cannot move
// the cursor or restyle the screen. Tabs become spaces.
func stripControl(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case isControl(r):
			return -1
		}
		return r
	}, s)
}

// trimURL drops punctuation that ends a sentence rather than the URL, and a
// closing parenthesis without its opening one.
func trimURL(u string) string {
	for u != "" {
		last := u[len(u)-1]
		switch {
		case strings.IndexByte(".,;:!?'\"", last) >= 0:
			u = u[:len(u)-1]
		case last == ')' && strings.Count(u, "(") < strings.Count(u, ")"):
			u = u[:len(u)-1]
		default:
			return u
		}
	}
	return u
}

// Links lists the distinct URLs in order of appearance.
func Links(segs []Segment) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range segs {
		if s.URL != "" && !seen[s.URL] {
			seen[s.URL] = true
			out = append(out, s.URL)
		}
	}
	return out
}

// cell is one grapheme with its segment's attributes.
type cell struct {
	g     string
	w     int
	attrs Segment
}

func cells(segs []Segment) []cell {
	var out []cell
	for _, s := range segs {
		state := -1
		rest := s.Text
		for rest != "" {
			var g string
			g, rest, _, state = uniseg.FirstGraphemeClusterInString(rest, state)
			out = append(out, cell{g: g, w: ansi.StringWidth(g), attrs: s})
		}
	}
	return out
}

func segments(cs []cell) []Segment {
	var out []Segment
	for _, c := range cs {
		a := c.attrs
		if n := len(out); n > 0 && out[n-1].Bold == a.Bold && out[n-1].Code == a.Code && out[n-1].URL == a.URL {
			out[n-1].Text += c.g
			continue
		}
		a.Text = c.g
		out = append(out, a)
	}
	return out
}

func isSpace(c cell) bool { return c.g == " " || c.g == "\t" }

// Wrap breaks segs into lines no wider than width, at spaces or around wide
// characters (CJK text has no spaces), cutting a word only when it does not
// fit on a line of its own. width < 1 means no wrapping.
func Wrap(segs []Segment, width int) [][]Segment {
	cs := cells(segs)
	if width < 1 {
		return [][]Segment{segments(cs)}
	}
	var lines [][]Segment
	for len(cs) > 0 {
		w, cut, brk := 0, len(cs), -1
		for i, c := range cs {
			if w+c.w > width {
				cut = i
				break
			}
			w += c.w
			// A break may go after a space, or before / after a wide character.
			if isSpace(c) || c.w > 1 {
				brk = i + 1
			}
			if i+1 < len(cs) && cs[i+1].w > 1 {
				brk = i + 1
			}
		}
		if cut < len(cs) && brk > 0 && !isSpace(cs[cut]) {
			cut = brk
		}
		if cut == 0 {
			cut = 1
		}
		line := cs[:cut]
		for len(line) > 0 && isSpace(line[len(line)-1]) {
			line = line[:len(line)-1]
		}
		lines = append(lines, segments(line))
		cs = cs[cut:]
		for len(cs) > 0 && isSpace(cs[0]) {
			cs = cs[1:]
		}
	}
	if len(lines) == 0 {
		lines = [][]Segment{nil}
	}
	return lines
}

// Truncate cuts segs to width, ending with "…" when something was cut.
func Truncate(segs []Segment, width int) []Segment {
	cs := cells(segs)
	total := 0
	for _, c := range cs {
		total += c.w
	}
	if total <= width || width < 1 {
		return segs
	}
	w, i := 0, 0
	for ; i < len(cs) && w+cs[i].w <= width-1; i++ {
		w += cs[i].w
	}
	out := segments(cs[:i])
	return append(out, Segment{Text: "…"})
}

var (
	boldStyle = lipgloss.NewStyle().Bold(true)
	codeStyle = lipgloss.NewStyle().Foreground(lipgloss.Cyan)
	linkStyle = lipgloss.NewStyle().Foreground(lipgloss.Blue).Underline(true)
)

const (
	osc8Close = "\x1b]8;;\x1b\\"
)

// Encode renders one line. Each link is opened and closed within the line, so
// a line can be drawn or cut on its own.
func Encode(segs []Segment) string {
	return EncodeWith(segs, lipgloss.NewStyle())
}

// EncodeWith renders one line on top of base (e.g. faint or reverse).
func EncodeWith(segs []Segment, base lipgloss.Style) string {
	var b strings.Builder
	plainBase := !hasStyle(base)
	for _, s := range segs {
		st := base
		styled := !plainBase
		if s.Bold {
			st = st.Inherit(boldStyle)
			styled = true
		}
		if s.Code {
			st = st.Inherit(codeStyle)
			styled = true
		}
		if s.URL != "" {
			st = st.Inherit(linkStyle)
			styled = true
			b.WriteString("\x1b]8;;" + s.URL + "\x1b\\")
		}
		if styled {
			b.WriteString(st.Render(s.Text))
		} else {
			b.WriteString(s.Text)
		}
		if s.URL != "" {
			b.WriteString(osc8Close)
		}
	}
	return b.String()
}

func hasStyle(s lipgloss.Style) bool {
	return s.Render("x") != "x"
}
