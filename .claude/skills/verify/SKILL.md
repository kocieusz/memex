---
name: verify
description: How to build and drive memex (a bubbletea TUI CLI) to verify changes end-to-end.
---

# Verifying memex

Build from the repo root: `task build` → `bin/memex`. Tests: `task test`.

memex is a bubbletea TUI, so drive it in an isolated tmux session and point
`MEMEX_SOURCE` at a temp dir so the real library (and any config file) is
never touched:

```sh
tmux -L memexv new-session -d -x 110 -y 30 \
  "MEMEX_SOURCE=/path/to/tmp-lib /path/to/bin/memex <cmd>; sleep 120"
tmux -L memexv send-keys j        # one key per send-keys call
tmux -L memexv capture-pane -p    # capture after each step
tmux -L memexv kill-server
```

Gotchas:
- Send keys one at a time with a short sleep between; batching several keys
  in one `send-keys` call (e.g. `send-keys ' ' Enter`) misfires.
- Space is `send-keys Space`, enter is `send-keys Enter`.
- For `memex clone`, `anthropics/skills` is a good public fixture (many
  skills, nested under `skills/`, plus a root-level `template`).
- Non-TUI paths (`ls`, error cases) run fine directly in Bash; check exit
  codes with `echo "exit=$?"`.
