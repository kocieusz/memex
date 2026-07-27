<div align="center">

<img src="assets/memex.png" alt="two retro computers linked by a cable, MEMEX · CONNECTED" width="720">

<h1><code>memex</code></h1>

Manage [Agent Skills](https://agentskills.io) across harnesses (Claude Code,
Codex, pi) with symlinks from one git-versioned library.

[Install](#install) · [Usage](#usage) · [Projects](#projects) · [Providers](#providers) · [Configuration](#configuration) · [Contributing](#contributing)

---

</div>

Named after Vannevar Bush's [memex](https://en.wikipedia.org/wiki/Memex), the
original vision of linked knowledge.

Skills live in one place (`~/.memex/skills` by default) and are linked as
directory symlinks into the global skills directory of each harness. memex only
ever creates and removes symlinks that point into the library; it never touches
real directories or links it doesn't own.

Every harness gets the same skills. You pick a set once and memex applies it to
every provider it finds. A **project** is a named set you can switch to:
`memex use writing` swaps the skills everywhere at once, `memex reset` puts the
default set back.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/kocieusz/memex/main/install.sh | sh
```

No Go toolchain required. The script:

1. detects your OS and CPU (macOS and Linux, arm64 and amd64) and downloads the
   matching binary from the [latest release](https://github.com/kocieusz/memex/releases/latest);
2. verifies it against the release's `checksums.txt` before installing;
3. installs a single binary at `~/.memex/bin/memex`, keeping memex self-contained
   alongside its library (`~/.memex/skills`) and config (`~/.memex/config.toml`);
4. creates the library at `~/.memex/skills` if it doesn't exist, so `memex` has
   somewhere to work from immediately;
5. adds `~/.memex/bin` to your `PATH` by appending this to your shell startup
   file (`~/.zshrc` for zsh, `~/.bash_profile` or `~/.bashrc` for bash):

   ```sh
   # added by memex install.sh
   export PATH="/Users/you/.memex/bin:$PATH"
   ```

   Then open a new terminal (or `source` that file) so the change takes effect.

If another `memex` sits earlier on your `PATH`, typically a leftover
`~/go/bin/memex` from a previous `go install`, the script warns you and tells you
how to remove it, so the old copy can't shadow the new one.

**Options** (environment variables):

| Variable | Effect |
| --- | --- |
| `MEMEX_INSTALL_DIR=~/.local/bin` | Install to a different directory (e.g. one already on your `PATH`). |
| `MEMEX_VERSION=v0.3.0` | Install a specific release instead of the latest. |
| `MEMEX_NO_MODIFY_PATH=1` | Don't touch your shell startup file; just print the `PATH` line to add yourself. |

### Updating

```sh
memex upgrade            # download the latest release and replace this binary
memex upgrade --check    # just report whether a newer release is available
```

`upgrade` verifies the checksum and swaps the running binary in place with an
atomic rename, so no stale second copy is left behind and an interrupted upgrade
leaves the working binary untouched. Run it any time.

### Install from source

With Go 1.26+, or on an OS/arch without a prebuilt binary:

```sh
go install github.com/kocieusz/memex@latest
```

The binary lands in `$(go env GOPATH)/bin` (usually `~/go/bin`), which is **not
on your `PATH` by default**:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"    # in ~/.zshrc
```

Update a `go install` build with `go install ...@latest` rather than
`memex upgrade`, which detects this case and points you back to the `go` command.

## Usage

Start a library by scaffolding a skill, pulling some from a repo, or adopting one
you already have:

```sh
memex touch my-skill                        # scaffold skills/my-skill/SKILL.md
memex clone anthropics/skills               # pick skills from a repo, copy them into the library
memex adopt ~/.agents/skills/some-skill     # move a real skill dir into the library, symlink back
```

Then run `memex` for the checklist. Space toggles a skill, enter applies it to
every provider at once:

```
  Project: default  Providers: claude, codex, pi
  Source: ~/.memex/skills (4 skills)

  ▸ [x] scoped-commits      linked
    [~] weighted-decision   will link in claude, pi  · blocked in codex
    [ ] data-normalisation  available

  ↑/↓ move · space toggle · a all · n none · / filter · enter apply · q quit
```

`[x]` is linked everywhere and `[~]` has providers still to catch up. A row is
blocked where something memex doesn't own, such as a real directory or someone
else's symlink, already holds that name. Applying links what you checked into
every provider and unlinks what you didn't, so the set on screen is exactly what
your harnesses end up with.

To re-apply the current set without the TUI, after installing a new harness or
pulling a library someone else added skills to:

```sh
memex sync
```

Inspect without the TUI:

```sh
memex ls                                    # your library, with each skill's origin repo
memex ls --target claude                    # linked/available/broken skills in a target
memex ls -a --target claude                 # also native dirs and foreign links
memex ls --target claude --json
```

Keep things healthy:

```sh
memex doctor                                # broken links, drift, sets naming skills you deleted
memex doctor --fix                          # remove the broken links and prune the stale names
```

`clone` shallow-clones the repo, finds every directory holding a `SKILL.md`, and
opens a checklist to pick the ones to copy. Each row shows the skill's path
inside the repo, and `i` reveals its description. It also takes full clone URLs
and GitHub `/tree/<branch>[/dir]` links; `--branch` picks a branch explicitly,
which is needed for branch names containing `/`.

memex records where each copied skill came from (repo, path, and a content hash)
in `.origins.toml` next to the skills. Re-running `memex clone` on the same repo
shows unchanged skills as `up to date` and changed ones as selectable updates,
warning you when your copy has local edits that updating would overwrite. Skills
that came from anywhere else can't be selected, including skills copied before
memex tracked origins; re-add those once to start tracking them.

## Projects

A project is a named set of library skills. Switching to one applies it to every
provider. Leaving it goes back to the default set, the one `memex` edits when no
project is active.

```sh
memex project new writing          # create it, then pick its skills in the checklist
memex use writing                  # apply it everywhere, and remember it
memex reset                        # back to the default set
memex project                      # pick which project to apply
memex project ls                   # every set, with the active one marked
memex project show writing         # the skills it holds
memex project edit writing         # change them without switching to it
memex project rm writing           # delete it; reset first if it's active
```

Projects live in `.projects.toml` inside the library, so a library kept in a git
repo carries its projects between machines. Which one is active is local to the
machine (`~/.memex/state.toml`), so one library can drive two computers
differently.

```toml
[default]
  skills = ["scoped-commits"]

[projects.writing]
  skills = ["citations", "prose-polish"]
```

The first time you use a project, the default set is seeded from whatever is
already linked, so adopting projects never unlinks the skills you had.

## Providers

Providers are the harnesses memex links into. Everything it detects is managed,
and a harness you install later is picked up automatically, so there is nothing
to opt into.

```sh
memex providers                    # claude, codex, pi, agents and their status
memex providers disable pi         # leave pi alone from now on
memex providers enable pi          # manage it again, then `memex sync`
```

Disabling a provider stops memex touching it; links already there stay put.

## Configuration

The library defaults to `~/.memex/skills`. To keep it somewhere else, say in your
dotfiles repo, point memex at it in `~/.memex/config.toml`:

```toml
library = "~/.dotfiles/skills"
disabled_providers = ["pi"]
```

Precedence for the library, highest first: the `--source` flag, the
`MEMEX_SOURCE` environment variable, the config file, the default.

`disabled_providers` is what `memex providers disable` writes. Edit it by hand if
you prefer: memex rewrites that one key and leaves the rest of the file, comments
included, alone.

## Contributing

memex doesn't accept external pull requests, so read
[CONTRIBUTING.md](CONTRIBUTING.md) before opening one. Bug reports and ideas are
very welcome as [issues](https://github.com/kocieusz/memex/issues).
