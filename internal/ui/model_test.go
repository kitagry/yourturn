package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/kitagry/yourturn/internal/actions"
)

const sample = `# demo

Reply with the number.

- **🟢 3 Later question?**
  - later body
- **🔴 1 Urgent question?**
  - urgent body line
  - [proj#12](https://example.com/proj/pull/12)
- **✅ 4 Done question**
- **🟠 2 Weekly question?**
  - see https://example.com/a and https://example.com/b
`

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "ctrl+d":
		return tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// harness drives a Model and records what it asked the outside world to do.
type harness struct {
	m       *Model
	opened  []string
	lastCmd tea.Cmd
}

// newTest builds a model that has loaded src at the given size.
func newTest(t *testing.T, src string, w, h int) *harness {
	t.Helper()
	hs := newHarness("/tmp/ACTIONS.md")
	hs = send(hs, tea.WindowSizeMsg{Width: w, Height: h})
	return send(hs, loadedMsg{doc: actions.Parse(src), modTime: time.Date(2026, 11, 22, 14, 56, 0, 0, time.Local)})
}

func newHarness(path string) *harness {
	hs := &harness{}
	hs.m = New(Options{Path: path, Open: func(url string) error {
		hs.opened = append(hs.opened, url)
		return nil
	}})
	return hs
}

func send(hs *harness, msg tea.Msg) *harness {
	next, cmd := hs.m.Update(msg)
	hs.m = next.(*Model)
	hs.lastCmd = cmd
	// Link opening runs as a command; run it so tests see the URL.
	if cmd != nil {
		// Only the link keys: other commands (ticks, cursor blink) block.
		if k, ok := msg.(tea.KeyPressMsg); ok && !hs.m.inputting && isLinkKey(k.String()) {
			if r, ok := cmd().(openedMsg); ok {
				hs.m.Update(r)
			}
		}
	}
	return hs
}

func screen(hs *harness) []string {
	return strings.Split(ansi.Strip(hs.m.View().Content), "\n")
}

func cursorID(hs *harness) string {
	it, ok := hs.m.current()
	if !ok {
		return ""
	}
	return it.ID
}

func TestGroupsByMarkAndHidesDone(t *testing.T) {
	m := newTest(t, sample, 60, 20)
	var ids []string
	for _, it := range m.m.visible() {
		ids = append(ids, it.ID)
	}
	if diff := cmp.Diff([]string{"1", "2", "3"}, ids); diff != "" {
		t.Errorf("visible order (-want +got):\n%s", diff)
	}
	m = send(m, key("a"))
	ids = nil
	for _, it := range m.m.visible() {
		ids = append(ids, it.ID)
	}
	if diff := cmp.Diff([]string{"1", "2", "3", "4"}, ids); diff != "" {
		t.Errorf("visible order with done (-want +got):\n%s", diff)
	}
}

func TestScreenShowsHeadingsAndCollapsedItems(t *testing.T) {
	m := newTest(t, sample, 60, 20)
	got := strings.Join(screen(m), "\n")
	for _, want := range []string{"demo", "open 3", "🔴 urgent", "Urgent question?", "🟠 this week", "Weekly question?", "🟢 when you have time"} {
		if !strings.Contains(got, want) {
			t.Errorf("screen lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "urgent body line") {
		t.Errorf("collapsed item shows its body:\n%s", got)
	}
	if strings.Contains(got, "Done question") {
		t.Errorf("done item shown by default:\n%s", got)
	}
}

func TestToggleAndMove(t *testing.T) {
	m := newTest(t, sample, 60, 20)
	if cursorID(m) != "1" {
		t.Fatalf("cursor = %q, want 1", cursorID(m))
	}
	m = send(m, key("enter"))
	if !strings.Contains(strings.Join(screen(m), "\n"), "urgent body line") {
		t.Errorf("enter did not open the item")
	}
	m = send(m, key("j"))
	if cursorID(m) != "2" {
		t.Errorf("after j cursor = %q, want 2", cursorID(m))
	}
	m = send(m, key("k"))
	m = send(m, key(" "))
	if strings.Contains(strings.Join(screen(m), "\n"), "urgent body line") {
		t.Errorf("space did not close the item")
	}
	m = send(m, key("E"))
	got := strings.Join(screen(m), "\n")
	if !strings.Contains(got, "urgent body line") || !strings.Contains(got, "later body") {
		t.Errorf("E did not open all:\n%s", got)
	}
	m = send(m, key("C"))
	if strings.Contains(strings.Join(screen(m), "\n"), "later body") {
		t.Errorf("C did not close all")
	}
	m = send(m, key("G"))
	if cursorID(m) != "3" {
		t.Errorf("after G cursor = %q, want 3", cursorID(m))
	}
	m = send(m, key("g"))
	if cursorID(m) != "1" {
		t.Errorf("after g cursor = %q, want 1", cursorID(m))
	}
}

func TestReloadKeepsCursorAndExpansion(t *testing.T) {
	m := newTest(t, sample, 60, 20)
	m = send(m, key("j"))
	m = send(m, key("enter"))
	// Item 1 is answered and removed; 2 must stay selected and open.
	changed := strings.Replace(sample, "- **🔴 1 Urgent question?**\n  - urgent body line\n  - [proj#12](https://example.com/proj/pull/12)\n", "", 1)
	m = send(m, loadedMsg{doc: actions.Parse(changed)})
	if cursorID(m) != "2" {
		t.Errorf("cursor = %q, want 2", cursorID(m))
	}
	if !m.m.expanded["2"] {
		t.Errorf("item 2 lost its expansion")
	}
	// The selected item disappears: the cursor stays at its position.
	m = send(m, loadedMsg{doc: actions.Parse(strings.Replace(changed, "🟠 2 Weekly", "✅ 2 Weekly", 1))})
	if cursorID(m) != "3" {
		t.Errorf("cursor = %q, want 3 after its item was done", cursorID(m))
	}
}

func TestCopyReplyPrefix(t *testing.T) {
	m := newTest(t, sample, 60, 20)
	m = send(m, key("y"))
	if m.lastCmd == nil {
		t.Fatal("y returned no command")
	}
	if !strings.Contains(m.m.status, "1 ") {
		t.Errorf("status = %q", m.m.status)
	}
}

func TestCopyLetteredReplyPrefix(t *testing.T) {
	for _, id := range []string{"gcp-ai-infra-maintenance#GM-24", "GCR-4"} {
		t.Run(id, func(t *testing.T) {
			m := newTest(t, "- **🔴 "+id+" Upgrade?**\n", 60, 20)
			m = send(m, key("y"))
			if m.lastCmd == nil {
				t.Fatalf("y returned no command; status = %q", m.m.status)
			}
			if diff := cmp.Diff(tea.SetClipboard(id+" ")(), m.lastCmd()); diff != "" {
				t.Errorf("y clipboard mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOpenLinks(t *testing.T) {
	m := newTest(t, sample, 60, 20)
	m = send(m, key("o"))
	if diff := cmp.Diff([]string{"https://example.com/proj/pull/12"}, m.opened); diff != "" {
		t.Errorf("opened (-want +got):\n%s", diff)
	}
	// Two links: pick the second by number.
	m = send(m, key("j"))
	m = send(m, key("o"))
	if len(m.m.picking) != 2 {
		t.Fatalf("picking = %v, want 2 links", m.m.picking)
	}
	if !strings.Contains(strings.Join(screen(m), "\n"), "2 https://example.com/b") {
		t.Errorf("link chooser not shown:\n%s", strings.Join(screen(m), "\n"))
	}
	m = send(m, key("2"))
	if diff := cmp.Diff([]string{"https://example.com/proj/pull/12", "https://example.com/b"}, m.opened); diff != "" {
		t.Errorf("opened (-want +got):\n%s", diff)
	}
	if m.m.picking != nil {
		t.Errorf("chooser still open")
	}
	m = send(m, key("j"))
	m = send(m, key("o"))
	if !strings.Contains(m.m.status, "no link") {
		t.Errorf("status = %q, want no link", m.m.status)
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		b.WriteString("- **🟢 " + itoa(i) + " Question " + itoa(i) + "**\n")
	}
	m := newTest(t, b.String(), 40, 10)
	for range 20 {
		m = send(m, key("j"))
	}
	if cursorID(m) != "21" {
		t.Fatalf("cursor = %q, want 21", cursorID(m))
	}
	got := strings.Join(screen(m), "\n")
	if !strings.Contains(got, "Question 21") {
		t.Errorf("cursor item not on screen:\n%s", got)
	}
	if lines := screen(m); len(lines) != 10 {
		t.Errorf("screen has %d lines, want 10", len(lines))
	}
	m = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	// The cursor follows the scroll so it is always on screen.
	found := false
	for _, l := range screen(m) {
		if strings.HasSuffix(strings.TrimRight(l, " "), "Question "+cursorID(m)) {
			found = true
		}
	}
	if !found {
		t.Errorf("cursor %s scrolled off screen:\n%s", cursorID(m), strings.Join(screen(m), "\n"))
	}
}

func TestOpenItemTallerThanScreenScrollsBeforeMoving(t *testing.T) {
	var b strings.Builder
	b.WriteString("- **🔴 1 Long?**\n")
	for i := range 20 {
		b.WriteString("  - line " + itoa(i) + "\n")
	}
	b.WriteString("- **🔴 2 Next?**\n")
	m := newTest(t, b.String(), 40, 8)
	m = send(m, key("enter"))
	m = send(m, key("j"))
	if cursorID(m) != "1" {
		t.Errorf("j left a long open item before showing its end: cursor = %q", cursorID(m))
	}
	for range 30 {
		m = send(m, key("j"))
	}
	if cursorID(m) != "2" {
		t.Errorf("cursor = %q, want 2 eventually", cursorID(m))
	}
}

func TestClickSelectsThenToggles(t *testing.T) {
	m := newTest(t, sample, 60, 20)
	row := -1
	for i, l := range screen(m) {
		if strings.Contains(l, "Weekly question?") {
			row = i
		}
	}
	if row < 0 {
		t.Fatal("weekly item not on screen")
	}
	m = send(m, tea.MouseClickMsg{Button: tea.MouseLeft, Y: row})
	if cursorID(m) != "2" {
		t.Fatalf("click did not select: cursor = %q", cursorID(m))
	}
	m = send(m, tea.MouseClickMsg{Button: tea.MouseLeft, Y: row})
	if !m.m.expanded["2"] {
		t.Errorf("second click did not open the item")
	}
}

func TestClickOnLinkChooserOpensThatLink(t *testing.T) {
	m := newTest(t, sample, 60, 20)
	m = send(m, key("j"))
	m = send(m, key("o"))
	row := -1
	for i, l := range screen(m) {
		if strings.HasPrefix(l, "2 https://example.com/b") {
			row = i
		}
	}
	if row < 0 {
		t.Fatalf("chooser row not on screen:\n%s", strings.Join(screen(m), "\n"))
	}
	m = send(m, tea.MouseClickMsg{Button: tea.MouseLeft, Y: row})
	if m.lastCmd == nil {
		t.Fatal("click on a chooser row returned no command")
	}
	if r, ok := m.lastCmd().(openedMsg); !ok || r.url != "https://example.com/b" {
		t.Errorf("click opened %v, want https://example.com/b", r)
	}
	if m.m.picking != nil || cursorID(m) != "2" {
		t.Errorf("chooser still open or cursor moved: picking=%v cursor=%q", m.m.picking, cursorID(m))
	}
}

func TestSelectedLongTitleWraps(t *testing.T) {
	src := "- **🟢 1 " + strings.Repeat("word ", 20) + "end?**\n- **🟢 2 " + strings.Repeat("other ", 20) + "tail?**\n"
	m := newTest(t, src, 30, 20)
	got := strings.Join(screen(m), "\n")
	if !strings.Contains(got, "end?") {
		t.Errorf("selected title not wrapped in full:\n%s", got)
	}
	if strings.Contains(got, "tail?") || !strings.Contains(got, "…") {
		t.Errorf("unselected title not truncated:\n%s", got)
	}
	for _, l := range screen(m) {
		if w := ansi.StringWidth(l); w > 30 {
			t.Errorf("line wider than the screen (%d): %q", w, l)
		}
	}
}

func TestUnparsedFileShowsRaw(t *testing.T) {
	m := newTest(t, "# nothing\n\n(no items yet)\n", 40, 10)
	if !strings.Contains(strings.Join(screen(m), "\n"), "(no items yet)") {
		t.Errorf("raw text not shown:\n%s", strings.Join(screen(m), "\n"))
	}
}

func TestJKScrollRawFile(t *testing.T) {
	var b strings.Builder
	for i := range 30 {
		b.WriteString("line " + itoa(i) + "\n\n")
	}
	m := newTest(t, b.String(), 40, 10)
	m = send(m, key("j"))
	if m.m.offset != 1 {
		t.Errorf("j on a raw file: offset = %d, want 1", m.m.offset)
	}
	m = send(m, key("k"))
	if m.m.offset != 0 {
		t.Errorf("k on a raw file: offset = %d, want 0", m.m.offset)
	}
}

func TestMissingFile(t *testing.T) {
	m := newHarness("/tmp/none/ACTIONS.md")
	m = send(m, tea.WindowSizeMsg{Width: 40, Height: 10})
	m = send(m, loadedMsg{missing: true})
	if !strings.Contains(strings.Join(screen(m), "\n"), "waiting for") {
		t.Errorf("missing file not explained:\n%s", strings.Join(screen(m), "\n"))
	}
}

func TestQuit(t *testing.T) {
	m := newTest(t, sample, 40, 10)
	m = send(m, key("q"))
	if m.lastCmd == nil {
		t.Fatal("q returned no command")
	}
	if _, ok := m.lastCmd().(tea.QuitMsg); !ok {
		t.Errorf("q did not quit")
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

func isLinkKey(k string) bool {
	return k == "o" || (len(k) == 1 && k[0] >= '1' && k[0] <= '9')
}

func TestMouseToggle(t *testing.T) {
	hs := newTest(t, sample, 80, 20)
	// The ui leaves the default to its caller: Options.Mouse is false here.
	if hs.m.View().MouseMode != tea.MouseModeNone {
		t.Error("mouse captured without Options.Mouse")
	}
	if !strings.Contains(screen(hs)[19], "m mouse off") {
		t.Errorf("footer does not show the mouse state: %q", screen(hs)[19])
	}
	send(hs, key("m"))
	if hs.m.View().MouseMode != tea.MouseModeCellMotion {
		t.Error("m did not capture the mouse")
	}
	if !strings.Contains(hs.m.status, "mouse on") {
		t.Errorf("status = %q", hs.m.status)
	}
	send(hs, key("j"))
	if !strings.Contains(screen(hs)[19], "m mouse on") {
		t.Errorf("footer does not show the mouse state: %q", screen(hs)[19])
	}
	send(hs, key("m"))
	if hs.m.View().MouseMode != tea.MouseModeNone {
		t.Error("m did not release the mouse")
	}
}

func TestMouseOption(t *testing.T) {
	m := New(Options{Path: "x", Mouse: true})
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Error("Mouse option ignored")
	}
}
