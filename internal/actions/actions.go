// Package actions parses ACTIONS.md: a Markdown list of things a human has to
// answer or do, one item per top-level list entry.
//
//   - **🔴 1 Merge PR #12?**
//   - background, options, links (indented lines)
//
// Parsing is lenient so that a file written by hand or by an agent that drifts
// from the format still shows up.
package actions

import (
	"regexp"
	"strings"
)

// Mark is an item's state, told by its emoji.
type Mark int

// The order is the order the viewer shows the groups in.
const (
	Urgent Mark = iota
	Week
	Later
	Waiting
	Other
	Done
)

var markEmoji = []struct {
	mark  Mark
	emoji string
}{
	{Urgent, "🔴"}, {Week, "🟠"}, {Later, "🟢"}, {Waiting, "⏳"}, {Done, "✅"},
}

// Emoji is the mark's emoji, "" for Other.
func (m Mark) Emoji() string {
	for _, e := range markEmoji {
		if e.mark == m {
			return e.emoji
		}
	}
	return ""
}

// Label is the group heading the viewer shows for the mark.
func (m Mark) Label() string {
	switch m {
	case Urgent:
		return "🔴 urgent"
	case Week:
		return "🟠 this week"
	case Later:
		return "🟢 when you have time"
	case Waiting:
		return "⏳ waiting"
	case Done:
		return "✅ done"
	default:
		return "other"
	}
}

// Open is whether the item still needs the human.
func (m Mark) Open() bool { return m != Done }

type Item struct {
	Mark Mark
	// ID is what a reply names the item by: "1", "M2" or "<task>#3". Empty
	// when the item has none.
	ID    string
	Title string
	// Body is the indented lines under the first line, with the item's own
	// indentation removed and deeper indentation kept.
	Body []string
}

// Key identifies the item across reloads.
func (it Item) Key() string {
	if it.ID != "" {
		return it.ID
	}
	return it.Title
}

// ReplyPrefix is how a reply to the item starts ("<ID> "), "" without an ID.
func (it Item) ReplyPrefix() string {
	if it.ID == "" {
		return ""
	}
	return it.ID + " "
}

type Doc struct {
	Title   string
	Preface []string
	Items   []Item
	// Parsed is false when no item was found; the viewer then shows Raw.
	Parsed bool
	Raw    string
}

var (
	listItem = regexp.MustCompile(`^[-*+]\s+(.*)$`)
	bold     = regexp.MustCompile(`^\*\*(.+?)\*\*\s*(.*)$`)
	itemID   = regexp.MustCompile(`^(\d+|M\d+|[a-z0-9][a-z0-9._-]*#\d+)\s+(.+)$`)
)

func Parse(src string) Doc {
	doc := Doc{Raw: src}
	section := Other
	var item *Item
	// pending holds blank lines inside an item; they are kept only if more
	// body follows.
	pending := 0
	indent := ""
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			if item != nil && len(item.Body) > 0 {
				pending++
			}
		case strings.HasPrefix(line, "## "):
			section = findMark(line, Other)
			item = nil
		case strings.HasPrefix(line, "# "):
			if doc.Title == "" && len(doc.Items) == 0 {
				doc.Title = strings.TrimSpace(line[2:])
			}
			item = nil
		case listItem.MatchString(line):
			doc.Items = append(doc.Items, parseItem(listItem.FindStringSubmatch(line)[1], section))
			item = &doc.Items[len(doc.Items)-1]
			pending, indent = 0, ""
		case item != nil && (line[0] == ' ' || line[0] == '\t'):
			if len(item.Body) == 0 {
				indent = line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			}
			for ; pending > 0; pending-- {
				item.Body = append(item.Body, "")
			}
			item.Body = append(item.Body, strings.TrimRight(strings.TrimPrefix(line, indent), " \t"))
		case len(doc.Items) == 0:
			doc.Preface = append(doc.Preface, trimmed)
		default:
			item = nil
		}
	}
	doc.Parsed = len(doc.Items) > 0
	return doc
}

func parseItem(text string, section Mark) Item {
	m := bold.FindStringSubmatch(text)
	if m == nil {
		return Item{Mark: leadingMark(text, section), Title: strings.TrimSpace(stripMark(text))}
	}
	it := Item{Mark: leadingMark(m[1], section)}
	head := strings.TrimSpace(stripMark(m[1]))
	if id := itemID.FindStringSubmatch(head); id != nil {
		it.ID, head = id[1], id[2]
	}
	it.Title = strings.TrimSpace(head + " " + m[2])
	return it
}

// findMark is the first mark emoji in s, or def without one.
func findMark(s string, def Mark) Mark {
	best, at := def, -1
	for _, e := range markEmoji {
		if i := strings.Index(s, e.emoji); i >= 0 && (at < 0 || i < at) {
			best, at = e.mark, i
		}
	}
	return best
}

// leadingMark is the mark emoji s starts with, or def without one. An item's
// mark leads its line; one later in the text (e.g. "CI ✅, merge?") is prose.
func leadingMark(s string, def Mark) Mark {
	s = strings.TrimSpace(s)
	for _, e := range markEmoji {
		if strings.HasPrefix(s, e.emoji) {
			return e.mark
		}
	}
	return def
}

// stripMark drops a leading mark emoji.
func stripMark(s string) string {
	s = strings.TrimSpace(s)
	for _, e := range markEmoji {
		if rest, ok := strings.CutPrefix(s, e.emoji); ok {
			return rest
		}
	}
	return s
}
