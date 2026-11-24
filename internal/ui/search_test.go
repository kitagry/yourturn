package ui

import (
	"strings"
	"testing"

	"github.com/kitagry/yourturn/internal/actions"
	"github.com/kitagry/yourturn/internal/render"
)

const searchSample = `# demo

- **🔴 1 Deploy the api?**
  - body one
- **🟠 2 Review docs**
  - mentions the API gateway
- **🟢 3 Lunch**
- **✅ 4 Old api thing**
`

func search(hs *harness, q string) {
	send(hs, key("/"))
	typeText(hs, q)
	send(hs, key("enter"))
}

// highlighted is how a match of s is drawn in a body line.
func highlighted(s string) string {
	return render.Encode([]render.Segment{{Text: s, Match: true}})
}

func TestSearchMovesThroughMatches(t *testing.T) {
	hs := newTest(t, searchSample, 60, 20)
	search(hs, "api")
	if got := cursorID(hs); got != "1" {
		t.Errorf("after / cursor = %q, want 1 (the current item matches)", got)
	}
	if !strings.Contains(hs.m.status, "1 of 2") {
		t.Errorf("status = %q, want the match position", hs.m.status)
	}
	send(hs, key("n"))
	if got := cursorID(hs); got != "2" {
		t.Errorf("after n cursor = %q, want 2", got)
	}
	if !hs.m.expanded["2"] {
		t.Error("item 2 matches only in its body but was not opened")
	}
	if hs.m.expanded["1"] {
		t.Error("item 1 matches in its title but was opened")
	}
	send(hs, key("n"))
	if got := cursorID(hs); got != "1" {
		t.Errorf("after second n cursor = %q, want 1", got)
	}
	if !strings.Contains(hs.m.status, "wrapped to the top") {
		t.Errorf("status = %q, want the wrap shown", hs.m.status)
	}
	send(hs, key("N"))
	if got := cursorID(hs); got != "2" {
		t.Errorf("after N cursor = %q, want 2", got)
	}
	if !strings.Contains(hs.m.status, "wrapped to the bottom") {
		t.Errorf("status = %q, want the wrap shown", hs.m.status)
	}
}

func TestSearchStartsAtTheCursor(t *testing.T) {
	hs := newTest(t, searchSample, 60, 20)
	send(hs, key("j"))
	search(hs, "deploy")
	if got := cursorID(hs); got != "1" {
		t.Errorf("cursor = %q, want 1 (wrapped from item 2)", got)
	}
}

func TestSearchHighlightsAndEscClears(t *testing.T) {
	hs := newTest(t, searchSample, 60, 20)
	search(hs, "gateway")
	if !strings.Contains(hs.m.View().Content, highlighted("gateway")) {
		t.Fatalf("match not highlighted:\n%s", hs.m.View().Content)
	}
	send(hs, key("esc"))
	if strings.Contains(hs.m.View().Content, highlighted("gateway")) {
		t.Error("esc left the highlight")
	}
	send(hs, key("n"))
	if !strings.Contains(hs.m.status, "/ to search") {
		t.Errorf("n after esc: status = %q", hs.m.status)
	}
}

func TestSearchInputKeepsKeys(t *testing.T) {
	hs := newTest(t, searchSample, 60, 20)
	send(hs, key("/"))
	if !strings.Contains(strings.Join(screen(hs), "\n"), "/") || !hs.m.inputting {
		t.Fatal("/ did not start the search input")
	}
	typeText(hs, "jqnR")
	if got := cursorID(hs); got != "1" {
		t.Errorf("typing moved the cursor to %q", got)
	}
	send(hs, key("esc"))
	if hs.m.inputting || hs.m.search != "" {
		t.Errorf("esc did not cancel: inputting=%v search=%q", hs.m.inputting, hs.m.search)
	}
}

func TestSearchEscKeepsPreviousSearch(t *testing.T) {
	hs := newTest(t, searchSample, 60, 20)
	search(hs, "api")
	send(hs, key("/"))
	typeText(hs, "lunch")
	send(hs, key("esc"))
	if hs.m.search != "api" {
		t.Errorf("search = %q, want the previous api kept", hs.m.search)
	}
}

func TestSearchNoMatch(t *testing.T) {
	hs := newTest(t, searchSample, 60, 20)
	send(hs, key("j"))
	search(hs, "zzz")
	if got := cursorID(hs); got != "2" {
		t.Errorf("cursor moved to %q", got)
	}
	if !strings.Contains(hs.m.status, `no match for "zzz"`) {
		t.Errorf("status = %q", hs.m.status)
	}
}

func TestSearchSkipsHiddenDoneItems(t *testing.T) {
	hs := newTest(t, searchSample, 60, 20)
	search(hs, "old")
	if !strings.Contains(hs.m.status, "no match") {
		t.Errorf("hidden done item matched: status = %q", hs.m.status)
	}
	send(hs, key("a"))
	send(hs, key("n"))
	if got := cursorID(hs); got != "4" {
		t.Errorf("with done shown cursor = %q, want 4", got)
	}
}

func TestSearchSurvivesReload(t *testing.T) {
	hs := newTest(t, searchSample, 60, 20)
	search(hs, "gateway")
	send(hs, loadedMsg{doc: actions.Parse(searchSample + "- **🟢 5 another gateway**\n"), seq: 1})
	if !strings.Contains(hs.m.View().Content, highlighted("gateway")) {
		t.Errorf("reload dropped the highlight:\n%s", hs.m.View().Content)
	}
	send(hs, key("n"))
	if got := cursorID(hs); got != "5" {
		t.Errorf("after reload n = %q, want the new item 5", got)
	}
}

func TestSearchHighlightsTheID(t *testing.T) {
	hs := newTest(t, "# demo\n\n- **🔴 CI-9 Fix the build**\n", 60, 20)
	search(hs, "ci")
	got := hs.m.View().Content
	if !strings.Contains(got, render.EncodeWith([]render.Segment{{Text: "CI", Bold: true, Match: true}}, idStyle.Inherit(selected))) {
		t.Errorf("the matched part of the ID is not highlighted:\n%q", got)
	}
	if !strings.Contains(got, idStyle.Inherit(selected).Render("-9")) {
		t.Errorf("the rest of the ID lost its color:\n%q", got)
	}
}

func TestSearchSeesAReloadHeldDuringInput(t *testing.T) {
	hs := newTest(t, searchSample, 60, 20)
	send(hs, key("/"))
	typeText(hs, "gateway")
	send(hs, loadedMsg{doc: actions.Parse(searchSample + "- **🟢 5 another gateway**\n"), seq: 1})
	send(hs, key("enter"))
	if !strings.Contains(hs.m.status, "1 of 2") {
		t.Errorf("status = %q, want both matches counted", hs.m.status)
	}
}
