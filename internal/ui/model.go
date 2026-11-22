// Package ui is the Bubble Tea model of the viewer.
package ui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kitagry/yourturn/internal/actions"
	"github.com/kitagry/yourturn/internal/render"
	"github.com/kitagry/yourturn/internal/watch"
)

type Options struct {
	Path string
	// Open opens a URL in the browser.
	Open func(url string) error
	// NoMouse leaves the mouse to the terminal, so clicking an OSC 8 link
	// opens it.
	NoMouse bool
	// Poll is the interval of the stat that backs up fsnotify.
	Poll time.Duration
}

type Model struct {
	path    string
	open    func(string) error
	noMouse bool
	poll    time.Duration
	changes <-chan struct{}

	doc     actions.Doc
	missing bool
	loadErr error
	modTime time.Time

	width, height int
	cursor        int
	// offset is the first body line on screen.
	offset   int
	expanded map[string]bool
	showDone bool
	help     bool
	// picking lists the links of the item when it has several, until one is
	// chosen.
	picking []string
	status  string

	// cache holds lines() for cacheKey; gen counts changes to what is drawn
	// other than the fields in the key.
	gen      int
	cache    []line
	cacheKey linesKey
}

type linesKey struct {
	gen, width, cursor int
	showDone           bool
}

func New(opts Options) *Model {
	poll := opts.Poll
	if poll <= 0 {
		poll = 5 * time.Second
	}
	return &Model{
		path:     opts.Path,
		open:     opts.Open,
		noMouse:  opts.NoMouse,
		poll:     poll,
		expanded: map[string]bool{},
	}
}

type loadedMsg struct {
	doc     actions.Doc
	modTime time.Time
	missing bool
	err     error
}

type changedMsg struct{}

type openedMsg struct {
	url string
	err error
}

func (m *Model) Init() tea.Cmd {
	m.changes = watch.Watch(context.Background(), m.path, m.poll)
	return tea.Batch(m.load(), m.waitChange())
}

func (m *Model) load() tea.Cmd {
	path := m.path
	return func() tea.Msg {
		b, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			return loadedMsg{missing: true}
		}
		if err != nil {
			return loadedMsg{err: err}
		}
		var mod time.Time
		if fi, err := os.Stat(path); err == nil {
			mod = fi.ModTime()
		}
		return loadedMsg{doc: actions.Parse(string(b)), modTime: mod}
	}
}

func (m *Model) waitChange() tea.Cmd {
	if m.changes == nil {
		return nil
	}
	ch := m.changes
	return func() tea.Msg {
		<-ch
		return changedMsg{}
	}
}

// visible is the items on screen, grouped by mark in display order.
func (m *Model) visible() []actions.Item {
	var out []actions.Item
	for _, it := range m.doc.Items {
		if m.showDone || it.Mark.Open() {
			out = append(out, it)
		}
	}
	slices.SortStableFunc(out, func(a, b actions.Item) int { return int(a.Mark) - int(b.Mark) })
	return out
}

func (m *Model) current() (actions.Item, bool) {
	items := m.visible()
	if m.cursor < 0 || m.cursor >= len(items) {
		return actions.Item{}, false
	}
	return items[m.cursor], true
}

func (m *Model) openCount() int {
	n := 0
	for _, it := range m.doc.Items {
		if it.Mark.Open() {
			n++
		}
	}
	return n
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampOffset()
	case loadedMsg:
		m.applyLoad(msg)
	case changedMsg:
		return m, tea.Batch(m.load(), m.waitChange())
	case openedMsg:
		if msg.err != nil {
			m.status = "open failed: " + msg.err.Error()
		} else {
			m.status = "opened " + msg.url
		}
	case tea.KeyPressMsg:
		return m, m.handleKey(msg)
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.scroll(-3)
		case tea.MouseWheelDown:
			m.scroll(3)
		}
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			return m, m.click(msg.Y)
		}
	}
	return m, nil
}

func (m *Model) applyLoad(msg loadedMsg) {
	if msg.err != nil {
		// Keep showing what was read last.
		m.loadErr = msg.err
		return
	}
	prevKey := ""
	if it, ok := m.current(); ok {
		prevKey = it.Key()
	}
	m.gen++
	m.loadErr, m.missing = nil, msg.missing
	m.picking = nil
	if msg.missing {
		m.doc = actions.Doc{}
		return
	}
	m.doc, m.modTime = msg.doc, msg.modTime
	items := m.visible()
	if i := slices.IndexFunc(items, func(it actions.Item) bool { return it.Key() == prevKey }); i >= 0 {
		m.cursor = i
	}
	m.cursor = min(m.cursor, max(len(items)-1, 0))
	m.ensureVisible()
}

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if m.picking != nil {
		links := m.picking
		m.picking = nil
		if n, err := strconv.Atoi(k); err == nil && n >= 1 && n <= len(links) {
			return m.openURL(links[n-1])
		}
		return nil
	}
	if m.help {
		m.help = false
		if k != "q" && k != "ctrl+c" {
			return nil
		}
	}
	m.status = ""
	switch k {
	case "q", "ctrl+c":
		return tea.Quit
	case "?":
		m.help = true
	case "j", "down":
		m.down()
	case "k", "up":
		m.up()
	case "ctrl+d":
		m.scroll(m.bodyHeight() / 2)
	case "ctrl+u":
		m.scroll(-m.bodyHeight() / 2)
	case "pgdown", "ctrl+f":
		m.scroll(m.bodyHeight())
	case "pgup", "ctrl+b":
		m.scroll(-m.bodyHeight())
	case "g", "home":
		m.cursor, m.offset = 0, 0
	case "G", "end":
		m.cursor = max(len(m.visible())-1, 0)
		m.offset = len(m.lines())
		m.clampOffset()
		m.ensureVisible()
	case "enter", " ", "space", "tab":
		m.setExpanded(func(open bool) bool { return !open })
	case "l", "right":
		m.setExpanded(func(bool) bool { return true })
	case "h", "left":
		m.setExpanded(func(bool) bool { return false })
	case "E":
		for _, it := range m.visible() {
			m.expanded[it.Key()] = true
		}
		m.gen++
		m.ensureVisible()
	case "C":
		clear(m.expanded)
		m.gen++
		m.clampOffset()
		m.ensureVisible()
	case "a":
		prev, _ := m.current()
		m.showDone = !m.showDone
		if i := slices.IndexFunc(m.visible(), func(it actions.Item) bool { return it.Key() == prev.Key() }); i >= 0 {
			m.cursor = i
		}
		m.cursor = min(m.cursor, max(len(m.visible())-1, 0))
		m.clampOffset()
		m.ensureVisible()
		if m.showDone {
			m.status = "showing done items"
		} else {
			m.status = "hiding done items"
		}
	case "o":
		it, ok := m.current()
		if !ok {
			return nil
		}
		links := itemLinks(it)
		switch len(links) {
		case 0:
			m.status = "no link in this item"
		case 1:
			return m.openURL(links[0])
		default:
			m.picking = links[:min(len(links), 9)]
		}
	case "y":
		it, ok := m.current()
		if !ok {
			return nil
		}
		prefix := it.ReplyPrefix()
		if prefix == "" {
			m.status = "this item has no number"
			return nil
		}
		m.status = fmt.Sprintf("copied %q", prefix)
		return tea.SetClipboard(prefix)
	case "r":
		return m.load()
	}
	return nil
}

func (m *Model) openURL(url string) tea.Cmd {
	open := m.open
	if open == nil {
		return nil
	}
	return func() tea.Msg {
		return openedMsg{url: url, err: open(url)}
	}
}

func itemLinks(it actions.Item) []string {
	segs := render.Inline(it.Title)
	for _, l := range it.Body {
		segs = append(segs, render.Inline(l)...)
	}
	return render.Links(segs)
}

func (m *Model) setExpanded(f func(bool) bool) {
	it, ok := m.current()
	if !ok {
		return
	}
	m.gen++
	if f(m.expanded[it.Key()]) {
		m.expanded[it.Key()] = true
	} else {
		delete(m.expanded, it.Key())
	}
	m.clampOffset()
	m.ensureVisible()
}

// down moves to the next item, but first scrolls through the rest of an open
// item that runs below the screen, so no line is skipped.
func (m *Model) down() {
	if len(m.visible()) == 0 {
		// Nothing to select (e.g. a file shown raw): move the screen instead.
		m.scroll(1)
		return
	}
	_, end := m.itemRange(m.cursor)
	if end > m.offset+m.bodyHeight() {
		m.offset++
		return
	}
	if m.cursor < len(m.visible())-1 {
		m.cursor++
		m.ensureVisible()
	}
}

func (m *Model) up() {
	if len(m.visible()) == 0 {
		m.scroll(-1)
		return
	}
	start, _ := m.itemRange(m.cursor)
	if start < m.offset {
		m.offset--
		return
	}
	if m.cursor > 0 {
		m.cursor--
		m.ensureVisible()
	}
}

// scroll moves the screen by n lines and keeps the cursor on an item that is
// on screen.
func (m *Model) scroll(n int) {
	m.offset += n
	m.clampOffset()
	items := len(m.visible())
	for i := range items {
		start, end := m.itemRange(i)
		if end > m.offset && start < m.offset+m.bodyHeight() {
			// The current item stays if it is still (partly) on screen.
			cs, ce := m.itemRange(m.cursor)
			if ce > m.offset && cs < m.offset+m.bodyHeight() {
				return
			}
			m.cursor = i
			if n < 0 {
				// Scrolling up: take the last item on screen instead.
				for j := items - 1; j >= i; j-- {
					s, _ := m.itemRange(j)
					if s < m.offset+m.bodyHeight() {
						m.cursor = j
						break
					}
				}
			}
			return
		}
	}
}

func (m *Model) click(y int) tea.Cmd {
	// An overlay covers the items, so a click goes to it, not to what is
	// underneath.
	if m.help {
		m.help = false
		return nil
	}
	if m.picking != nil {
		links := m.picking
		m.picking = nil
		// Matches body(): the chooser's heading and links take the last rows.
		shown := min(len(links)+1, m.bodyHeight())
		if n := y - (m.bodyHeight() - shown + 1); n >= 1 && n < shown {
			return m.openURL(links[n-1])
		}
		return nil
	}
	row := y - 1 + m.offset
	lines := m.lines()
	if y < 1 || y > m.bodyHeight() || row >= len(lines) || lines[row].item < 0 {
		return nil
	}
	if lines[row].item == m.cursor {
		m.setExpanded(func(open bool) bool { return !open })
		return nil
	}
	m.cursor = lines[row].item
	m.ensureVisible()
	return nil
}

// bodyHeight is the rows between the header and the footer.
func (m *Model) bodyHeight() int {
	return max(m.height-2, 1)
}

func (m *Model) clampOffset() {
	m.offset = min(m.offset, max(len(m.lines())-m.bodyHeight(), 0))
	m.offset = max(m.offset, 0)
}

// itemRange is the body lines [start, end) of the i-th visible item.
func (m *Model) itemRange(i int) (int, int) {
	start, end := -1, -1
	for n, l := range m.lines() {
		if l.item == i {
			if start < 0 {
				start = n
			}
			end = n + 1
		}
	}
	return start, end
}

// ensureVisible scrolls so the cursor's item is on screen: all of it if it
// fits, else its top. The group heading above it comes along when it fits.
func (m *Model) ensureVisible() {
	start, end := m.itemRange(m.cursor)
	if start < 0 {
		return
	}
	h := m.bodyHeight()
	lines := m.lines()
	top := start
	if top > 0 && lines[top-1].heading && end-(top-1) <= h {
		top--
	}
	switch {
	case top < m.offset:
		m.offset = top
	case end > m.offset+h:
		m.offset = min(top, end-h)
	}
	m.clampOffset()
}

// line is one rendered body line; item is the visible item it belongs to,
// -1 for headings and text.
type line struct {
	text    string
	item    int
	heading bool
}

var (
	faint       = lipgloss.NewStyle().Faint(true)
	headStyle   = lipgloss.NewStyle().Bold(true)
	idStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Yellow)
	selected    = lipgloss.NewStyle().Reverse(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Red)
	pointer     = lipgloss.NewStyle().Foreground(lipgloss.Magenta).Bold(true)
	titleStyle  = lipgloss.NewStyle().Bold(true)
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Green)
)

func (m *Model) lines() []line {
	key := linesKey{gen: m.gen, width: m.width, cursor: m.cursor, showDone: m.showDone}
	if m.cache != nil && key == m.cacheKey {
		return m.cache
	}
	m.cache, m.cacheKey = m.renderLines(), key
	return m.cache
}

func (m *Model) renderLines() []line {
	w := max(m.width, 10)
	var out []line
	text := func(s string, st lipgloss.Style) {
		for _, l := range render.Wrap(render.Inline(s), w) {
			out = append(out, line{text: render.EncodeWith(l, st), item: -1})
		}
	}
	if !m.doc.Parsed {
		for _, l := range strings.Split(strings.TrimRight(m.doc.Raw, "\n"), "\n") {
			text(l, lipgloss.NewStyle())
		}
		return out
	}
	for _, p := range m.doc.Preface {
		text(p, faint)
	}
	items := m.visible()
	counts := map[actions.Mark]int{}
	for _, it := range items {
		counts[it.Mark]++
	}
	group := actions.Mark(-1)
	for i, it := range items {
		if it.Mark != group {
			group = it.Mark
			n := counts[group]
			label := fmt.Sprintf("── %s %d ", group.Label(), n)
			out = append(out, line{text: headStyle.Render(label + strings.Repeat("─", max(w-lipgloss.Width(label), 0))), item: -1, heading: true})
		}
		out = append(out, m.itemLines(i, it, w)...)
	}
	if len(items) == 0 {
		text("(nothing to do)", faint)
	}
	return out
}

func (m *Model) itemLines(i int, it actions.Item, w int) []line {
	open := m.expanded[it.Key()]
	isCur := i == m.cursor
	hasBody := len(it.Body) > 0
	marker := "  "
	switch {
	case isCur:
		marker = pointer.Render(markerRune(open, hasBody)) + " "
	case open || hasBody:
		marker = markerRune(open, hasBody) + " "
	}
	var segs []render.Segment
	if it.ID != "" {
		segs = append(segs, render.Segment{Text: it.ID, Bold: true}, render.Segment{Text: " "})
	}
	segs = append(segs, render.Inline(it.Title)...)
	base := lipgloss.NewStyle()
	if !it.Mark.Open() {
		base = faint
	}
	if isCur {
		base = base.Inherit(selected)
	}
	var title [][]render.Segment
	if isCur || open {
		title = render.Wrap(segs, w-2)
	} else {
		title = [][]render.Segment{render.Truncate(segs, w-2)}
	}
	var out []line
	for n, l := range title {
		prefix := "  "
		if n == 0 {
			prefix = marker
		}
		out = append(out, line{text: prefix + encodeTitle(l, it.ID, n == 0, base), item: i})
	}
	if !open {
		return out
	}
	for _, b := range it.Body {
		out = append(out, bodyLines(b, w, i)...)
	}
	return out
}

func markerRune(open, hasBody bool) string {
	switch {
	case open:
		return "▾"
	case hasBody:
		return "▸"
	default:
		return "›"
	}
}

// encodeTitle renders a title line, coloring the leading ID on its first line.
func encodeTitle(l []render.Segment, id string, first bool, base lipgloss.Style) string {
	if first && id != "" && len(l) > 0 && l[0].Text == id {
		return idStyle.Inherit(base).Render(id) + render.EncodeWith(l[1:], base)
	}
	return render.EncodeWith(l, base)
}

// bodyLines renders one body line: list markers become bullets and wrapped
// lines hang under the text, keeping the source's nesting.
func bodyLines(src string, w, item int) []line {
	if strings.TrimSpace(src) == "" {
		return []line{{item: item}}
	}
	trimmed := strings.TrimLeft(src, " \t")
	indent := 4 + (len(src) - len(trimmed))
	bullet := ""
	for _, mk := range []string{"- ", "* ", "+ "} {
		if rest, ok := strings.CutPrefix(trimmed, mk); ok {
			bullet, trimmed = "• ", rest
			break
		}
	}
	hang := indent + lipgloss.Width(bullet)
	var out []line
	for n, l := range render.Wrap(render.Inline(trimmed), max(w-hang, 4)) {
		lead := strings.Repeat(" ", hang)
		if n == 0 {
			lead = strings.Repeat(" ", indent) + faint.Render(bullet)
		}
		out = append(out, line{text: lead + render.Encode(l), item: item})
	}
	return out
}

func (m *Model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	if !m.noMouse {
		v.MouseMode = tea.MouseModeCellMotion
	}
	if m.width == 0 || m.height == 0 {
		return v
	}
	rows := make([]string, 0, m.height)
	rows = append(rows, m.header())
	body := m.body()
	rows = append(rows, body...)
	for len(rows) < m.height-1 {
		rows = append(rows, "")
	}
	if m.height > 1 {
		rows = append(rows, m.footer())
	}
	v.SetContent(strings.Join(rows, "\n"))
	return v
}

func (m *Model) body() []string {
	h := m.bodyHeight()
	if m.help {
		return helpLines(h)
	}
	if m.missing {
		return []string{faint.Render(fit("waiting for "+m.path, m.width))}
	}
	lines := m.lines()
	var out []string
	for i := m.offset; i < len(lines) && len(out) < h; i++ {
		out = append(out, lines[i].text)
	}
	if m.picking != nil {
		// The chooser takes the bottom rows of the body.
		var pick []string
		pick = append(pick, headStyle.Render("open which link? (1-"+strconv.Itoa(len(m.picking))+", esc to cancel)"))
		for n, u := range m.picking {
			pick = append(pick, fit(strconv.Itoa(n+1)+" "+u, m.width))
		}
		pick = pick[:min(len(pick), h)]
		for len(out) < h {
			out = append(out, "")
		}
		out = append(out[:h-len(pick)], pick...)
	}
	return out
}

func (m *Model) header() string {
	title := m.doc.Title
	if title == "" {
		title = filepath.Base(m.path)
	}
	right := fmt.Sprintf("open %d", m.openCount())
	if !m.modTime.IsZero() {
		right += " · " + m.modTime.Format("15:04")
	}
	room := m.width - lipgloss.Width(right) - 1
	if room < 4 {
		return fit(right, m.width)
	}
	left := fit(title, room)
	return titleStyle.Render(left) + strings.Repeat(" ", max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 1)) + faint.Render(right)
}

func (m *Model) footer() string {
	switch {
	case m.loadErr != nil:
		return errStyle.Render(fit("error: "+m.loadErr.Error(), m.width))
	case m.status != "":
		return statusStyle.Render(fit(m.status, m.width))
	}
	return faint.Render(fit("j/k move · enter open · o link · y copy id · a done · ? help", m.width))
}

func helpLines(h int) []string {
	lines := []string{
		"j/k ↓/↑     move (scrolls through a long open item first)",
		"ctrl-d/u    half page down / up",
		"pgdn/pgup   page down / up",
		"g/G         first / last",
		"enter space open / close",
		"l/h         open / close",
		"E/C         open all / close all",
		"a           show / hide done (✅) items",
		"o           open the item's link",
		"y           copy the item's number to reply with",
		"r           reload",
		"mouse       wheel scrolls, click selects, click again opens",
		"q           quit",
		"",
		"press any key to go back",
	}
	return lines[:min(len(lines), h)]
}

// fit cuts s to width columns.
func fit(s string, width int) string {
	return render.Encode(render.Truncate(render.Inline(s), width))
}
