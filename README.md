<div align="center">

<img src="assets/memex.png" alt="two retro computers linked by a cable — MEMEX · CONNECTED" width="720">

<h1><code>memex</code></h1>

Manage [Agent Skills](https://agentskills.io) across harnesses — Claude Code,
Codex, pi — with symlinks from one git-versioned library.

[Install](#install) · [Usage](#usage) · [Configuration](#configuration) · [Contributing](#contributing)

---

</div>

Named after Vannevar Bush's [memex](https://en.wikipedia.org/wiki/Memex), the
original vision of linked knowledge.

Skills live in one place (`~/.memex/skills` by default) and are linked as
directory symlinks into the global skills directory of each harness. memex
only ever creates and removes symlinks that point into the library; it never
touches real directories or links it doesn't own.

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

Then run `memex` to pick a harness target (claude, codex, pi, agents) and get
an interactive checklist — space toggles a skill, enter applies:

```
  Target: ~/.claude/skills          Source: ~/.memex/skills (2 skills)

  ▸ [x] scoped-commits        linked
    [ ] weighted-decision     available

  ↑/↓ move · space toggle · a all · n none · / filter · enter apply · q quit
```

After applying (or backing out with `q`), you return to the picker, so
several harnesses can be updated in one session.

Inspect without the TUI:

```sh
memex ls                                    # your library, with each skill's origin repo
memex ls --target claude                    # linked/available/broken skills in a target
memex ls -a --target claude                 # also native dirs and foreign links
memex ls --target claude --json
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
your dotfiles repo — point memex at it in `~/.memex/config.toml`:

```toml
library = "~/.dotfiles/skills"
```

Precedence, highest first: the `--source` flag, the `MEMEX_SOURCE` environment
variable, the config file, the default.

## Contributing

memex doesn't accept external pull requests — read
[CONTRIBUTING.md](CONTRIBUTING.md) before opening one. Bug reports and ideas
are very welcome as [issues](https://github.com/kocieusz/memex/issues).
