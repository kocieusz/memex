<div align="center">

<img src="assets/memex.png" alt="two retro computers linked by a cable — MEMEX · CONNECTED" width="720">

<h1><code>memex</code></h1>

Manage [Agent Skills](https://agentskills.io) across harnesses — Claude Code,
Codex, pi — with symlinks from one git-versioned library.

[Install](#install) · [Usage](#usage) · [Configuration](#configuration) · [Development](#development)

---

</div>

Named after Vannevar Bush's [memex](https://en.wikipedia.org/wiki/Memex), the
original vision of linked knowledge.

Skills live in one place (`~/.memex/skills` by default) and are linked as
directory symlinks into any skills directory a harness reads — global or
per-project. memex only ever creates and removes symlinks that point into the
library; it never touches real directories or links it doesn't own.

## Install

Requires Go 1.26+:

```sh
go install github.com/kocieusz/memex@latest
```

The binary lands in `$(go env GOPATH)/bin` (usually `~/go/bin`) — make sure
that's on your `PATH`.

## Usage

Start a library by scaffolding a skill, pulling some from a repo, or adopting
one you already have:

```sh
memex touch my-skill                        # scaffold skills/my-skill/SKILL.md
memex clone anthropics/skills               # pick skills from a repo, copy them into the library
memex adopt ~/.agents/skills/some-skill     # move a real skill dir into the library, symlink back
```

Then run `memex` inside any skills directory (or a project containing one) to
get an interactive checklist — space toggles a skill, enter applies:

```
  Target: ~/.claude/skills          Source: ~/.memex/skills (2 skills)

  ▸ [x] scoped-commits        linked
    [ ] weighted-decision     available

  ↑/↓ move · space toggle · a all · n none · / filter · enter apply · q quit
```

Run it outside a skills directory to pick a global harness target (claude,
codex, pi, agents) — all linking and unlinking happens through the TUI. After
applying (or backing out with `q`), you return to the picker, so several
harnesses can be updated in one session. Inside a project, `memex global`
skips the project's own skills dirs and goes straight to the global targets.

When the target lives inside a project's git repo, memex maintains a
`.gitignore` next to the symlinks: linked skills are added (they point into
your home dir and would be broken for collaborators) and removed again on
unlink, deleting the file once nothing else remains in it.

Inspect without the TUI:

```sh
memex ls --target claude                    # linked/available/broken skills
memex ls -a --target claude                 # also native dirs and foreign links
memex ls --target claude --json
memex ls                                    # in a skills dir: that dir; in the library: all your skills
```

Keep things healthy:

```sh
memex doctor --fix                          # remove broken links, report missing SKILL.md
```

`clone` shallow-clones the repo, finds every directory holding a `SKILL.md`,
and opens a checklist to pick the ones to copy; each row shows the skill's
path inside the repo, and `i` reveals its description. It also takes full
clone URLs and GitHub `/tree/<branch>[/dir]` links; `--branch` picks a branch
explicitly (needed for branch names containing `/`). Skills whose name
already exists in the library are shown but can't be selected.

## Configuration

The library defaults to `~/.memex/skills`. To keep it somewhere else — say, in
your dotfiles repo — point memex at it in `~/.config/memex/config.toml`
(`$XDG_CONFIG_HOME` is honoured):

```toml
library = "~/.dotfiles/skills"
```

Precedence, highest first: the `--source` flag, the `MEMEX_SOURCE` environment
variable, the config file, the default.

## Development

Uses [Task](https://taskfile.dev):

```sh
task         # list available tasks
task build   # build bin/memex
task test    # run the test suite
```

Layout: `main.go` wires the [kong](https://github.com/alecthomas/kong) CLI;
`internal/config` loads the config file, `internal/library` scans the skill
library, `internal/target` classifies target entries and performs all linking,
`internal/remote` handles `clone`, and `internal/tui` is the
[bubbletea](https://github.com/charmbracelet/bubbletea) checklist.
