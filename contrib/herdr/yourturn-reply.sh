#!/usr/bin/env bash
# Sends a reply typed in yourturn to the herdr-task setup.
#
#   yourturn --reply-cmd ~/path/to/yourturn-reply.sh ~/tasks/ACTIONS.md
#
# yourturn passes the reply in YOURTURN_ID / YOURTURN_TEXT / YOURTURN_FILE.
# The ID is "<task>#<n>" in the master's aggregated ACTIONS.md, "<n>" in a
# task's own ACTIONS.md (the task is the file's directory under the tasks
# root) and "M<n>" for the master's own items. <n> is digits, optionally
# after upper-case letters and a "-" ("3", "A266", "CI-9").
#
# YOURTURN_HERDR_MODE picks where it goes:
#   inbox (default)  append "[yourturn] human reply: <task>#<n> <text>" to
#                    <tasks root>/.inbox. The master wakes on the inbox and
#                    passes the reply on like one the human typed, so it
#                    knows every answer, and nothing is typed into a pane.
#   coordinator      send "[yourturn] <n> <text>" straight to the task's
#                    coordinator with task-ask.sh --no-wait, and copy it to
#                    the inbox. M items have no coordinator and go to the
#                    inbox.
#
# HERDR_TASKS_ROOT (default ~/tasks) and HERDR_TASK_SCRIPTS (default
# ~/.claude/skills/herdr-task/scripts) locate the setup.
set -euo pipefail

root="${HERDR_TASKS_ROOT:-$HOME/tasks}"
scripts="${HERDR_TASK_SCRIPTS:-$HOME/.claude/skills/herdr-task/scripts}"
mode="${YOURTURN_HERDR_MODE:-inbox}"
id="${YOURTURN_ID:?YOURTURN_ID is not set (run from yourturn --reply-cmd)}"
# The inbox is one line per entry.
text=$(printf '%s' "${YOURTURN_TEXT:-}" | tr '\n\t' '  ')
file="${YOURTURN_FILE:-}"

task="" num=""
if [[ "$id" =~ ^([a-z0-9][a-z0-9._-]*)#(([A-Z]+-?)?[0-9]+)$ ]]; then
  task="${BASH_REMATCH[1]}" num="${BASH_REMATCH[2]}"
elif [[ "$id" =~ ^([A-Z]+-?)?[0-9]+$ ]]; then
  dir=$(dirname "$file")
  # Both are empty when neither directory exists, which must not match.
  parent=$(cd "$(dirname "$dir")" 2>/dev/null && pwd -P) || parent=""
  root_real=$(cd "$root" 2>/dev/null && pwd -P) || root_real=""
  if [ -n "$root_real" ] && [ "$parent" = "$root_real" ]; then
    # Checked before M<n>: master's items never live in a task's file, so
    # "M12" there is the task's own lettered number.
    task=$(basename "$dir") num="$id"
  elif [[ "$id" =~ ^M[0-9]+$ ]]; then
    task=""
  else
    echo "cannot tell the task of item $id: $file is not in a task directory under $root" >&2
    exit 2
  fi
else
  echo "unknown item ID: $id" >&2
  exit 2
fi

to_inbox() {
  printf '%s\t%s\t%s\n' "$(date '+%F %T')" "${task:-master}" "$1" >> "$root/.inbox"
}

qualified="${task:+$task#$num}"
qualified="${qualified:-$id}"

if [ "$mode" = coordinator ] && [ -n "$task" ]; then
  status=0
  "$scripts/task-ask.sh" "$task" "[yourturn] $num $text" --no-wait > /dev/null || status=$?
  case "$status" in
    0) ;;
    4) echo "the coordinator of $task is waiting on a prompt in its tab; answer it there" >&2; exit 4 ;;
    *) echo "task-ask.sh failed for $task (exit $status)" >&2; exit "$status" ;;
  esac
  to_inbox "[yourturn] human reply (already sent to the coordinator): $qualified $text"
  exit 0
fi

case "$mode" in
  inbox | coordinator) to_inbox "[yourturn] human reply: $qualified $text" ;;
  *) echo "unknown YOURTURN_HERDR_MODE: $mode (inbox or coordinator)" >&2; exit 2 ;;
esac
