package ui

import (
	"os"
	"strings"
	"time"
)

// The states a reply goes through after it is sent, in order. "done" is not
// among them: it is read off the item (checked off or gone), not reported.
const (
	statusRelayed = "relayed"
	statusAcked   = "acked"
)

// replyEvents is what the reply status file says, by the reply's receipt and
// then by state.
type replyEvents map[string]map[string]time.Time

// readReplyStatus reads a file of lines "<receipt>\t<state>\t<RFC3339 time>",
// appended by whatever carries the reply on. Lines that are broken or name a
// state that is not known are skipped, so a newer writer does not break an
// older viewer. When a state is reported twice the first time is kept.
func readReplyStatus(path string) replyEvents {
	events := replyEvents{}
	b, err := os.ReadFile(path)
	if err != nil {
		return events
	}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Split(strings.TrimRight(l, "\r"), "\t")
		if len(f) != 3 || f[0] == "" || (f[1] != statusRelayed && f[1] != statusAcked) {
			continue
		}
		at, err := time.Parse(time.RFC3339, f[2])
		if err != nil {
			continue
		}
		if events[f[0]] == nil {
			events[f[0]] = map[string]time.Time{}
		}
		if old, ok := events[f[0]][f[1]]; !ok || at.Before(old) {
			events[f[0]][f[1]] = at
		}
	}
	return events
}
