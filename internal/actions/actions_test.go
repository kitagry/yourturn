package actions

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	tests := map[string]struct {
		src  string
		want Doc
	}{
		"task file": {
			src: `# demo actions

Answer with the number, e.g. "1 OK".

- **🔴 1 Merge PR #12?**
  - it ships to prod
  - https://example.com/pull/12
- **🟢 2 A or B?**
  - A: keep / B: drop
    - nested detail
- **✅ 3 Done thing**
`,
			want: Doc{
				Title:   "demo actions",
				Preface: []string{`Answer with the number, e.g. "1 OK".`},
				Parsed:  true,
				Items: []Item{
					{Mark: Urgent, ID: "1", Title: "Merge PR #12?", Body: []string{"- it ships to prod", "- https://example.com/pull/12"}},
					{Mark: Later, ID: "2", Title: "A or B?", Body: []string{"- A: keep / B: drop", "  - nested detail"}},
					{Mark: Done, ID: "3", Title: "Done thing"},
				},
			},
		},
		"aggregated file groups by heading but item marks win": {
			src: `# todo

## 🟠
- **🟠 alpha#36 Pick one?**
  - detail
- **🟠 M2 Master's own item**

## ⏳
- **beta-tools#4 no mark in line**
`,
			want: Doc{
				Title:  "todo",
				Parsed: true,
				Items: []Item{
					{Mark: Week, ID: "alpha#36", Title: "Pick one?", Body: []string{"- detail"}},
					{Mark: Week, ID: "M2", Title: "Master's own item"},
					{Mark: Waiting, ID: "beta-tools#4", Title: "no mark in line"},
				},
			},
		},
		"numbers with letters": {
			src: `- **🔴 ci-maintenance#CI-9 Fix CI?**
- **🔴 system-admin-api-go#SAG-56 Review?**
- **🟠 patient-api-go#A266 Pick one?**
- **🟠 gcp-ai-infra-maintenance#GM-24 Upgrade?**
- **🟢 SAG-56 Bare lettered number**
- **🟢 A266 Bare letter and digits**
`,
			want: Doc{
				Parsed: true,
				Items: []Item{
					{Mark: Urgent, ID: "ci-maintenance#CI-9", Title: "Fix CI?"},
					{Mark: Urgent, ID: "system-admin-api-go#SAG-56", Title: "Review?"},
					{Mark: Week, ID: "patient-api-go#A266", Title: "Pick one?"},
					{Mark: Week, ID: "gcp-ai-infra-maintenance#GM-24", Title: "Upgrade?"},
					{Mark: Later, ID: "SAG-56", Title: "Bare lettered number"},
					{Mark: Later, ID: "A266", Title: "Bare letter and digits"},
				},
			},
		},
		"words that only look like ids are titles": {
			src: `- **🟢 GM-24/25 Both of them**
- **🟢 task#gm-24 Lowercase letters**
- **🟢 CI- Dash without digits**
- **🟢 CI-9-1 Two dashes**
`,
			want: Doc{
				Parsed: true,
				Items: []Item{
					{Mark: Later, Title: "GM-24/25 Both of them"},
					{Mark: Later, Title: "task#gm-24 Lowercase letters"},
					{Mark: Later, Title: "CI- Dash without digits"},
					{Mark: Later, Title: "CI-9-1 Two dashes"},
				},
			},
		},
		"text after the bold part and items without bold or id": {
			src: `- **🟢 Check it** — more words
- plain item
  continuation
`,
			want: Doc{
				Parsed: true,
				Items: []Item{
					{Mark: Later, Title: "Check it — more words"},
					{Mark: Other, Title: "plain item", Body: []string{"continuation"}},
				},
			},
		},
		"blank lines inside an item are kept between body lines": {
			src: "- **🔴 1 Q?**\n  - a\n\n  - b\n\n- **🔴 2 R?**\n",
			want: Doc{
				Parsed: true,
				Items: []Item{
					{Mark: Urgent, ID: "1", Title: "Q?", Body: []string{"- a", "", "- b"}},
					{Mark: Urgent, ID: "2", Title: "R?"},
				},
			},
		},
		"a mark later in the line does not set the item's mark": {
			src: "## 🔴\n- **1 CI ✅, merge?**\n- plain ✅ mention\n",
			want: Doc{
				Parsed: true,
				Items: []Item{
					{Mark: Urgent, ID: "1", Title: "CI ✅, merge?"},
					{Mark: Urgent, Title: "plain ✅ mention"},
				},
			},
		},
		"no items falls back to raw text": {
			src:  "# nothing\n\n(none yet)\n",
			want: Doc{Title: "nothing", Preface: []string{"(none yet)"}},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := Parse(tt.src)
			tt.want.Raw = tt.src
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestItemKey(t *testing.T) {
	if got := (Item{ID: "a#1", Title: "x"}).Key(); got != "a#1" {
		t.Errorf("Key() = %q, want a#1", got)
	}
	if got := (Item{Title: "x"}).Key(); got != "x" {
		t.Errorf("Key() = %q, want x", got)
	}
}

func TestReplyPrefix(t *testing.T) {
	if got := (Item{ID: "a#1"}).ReplyPrefix(); got != "a#1 " {
		t.Errorf("ReplyPrefix() = %q", got)
	}
	if got := (Item{Title: "x"}).ReplyPrefix(); got != "" {
		t.Errorf("ReplyPrefix() = %q, want empty", got)
	}
}
