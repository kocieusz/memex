// Package remote fetches skills from a git repository: parse a repo
// reference, shallow-clone it, discover the skills inside, and copy the
// selected ones into the library.
package remote

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Ref identifies what to clone and where to look inside it.
type Ref struct {
	URL    string // git clone URL
	Name   string // repository name; names a skill sitting at the repo root
	Branch string // empty means the default branch
	Subdir string // repo-relative dir to discover under; empty means the root
}

var shorthandRe = regexp.MustCompile(`^[a-zA-Z0-9][\w.-]*/[\w.-]+$`)

// ParseRef understands owner/repo shorthand (assumed GitHub), ssh and https
// clone URLs, and GitHub /tree/<branch>[/<dir>] links. A branch name that
// itself contains slashes defeats the /tree/ heuristic (only the first
// segment is taken as the branch) — pass the plain repo URL with --branch
// for those.
func ParseRef(arg string) (Ref, error) {
	if shorthandRe.MatchString(arg) {
		return Ref{URL: "https://github.com/" + arg, Name: repoName(arg)}, nil
	}
	if strings.HasPrefix(arg, "git@") {
		return Ref{URL: arg, Name: repoName(arg)}, nil
	}
	u, err := url.Parse(arg)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return Ref{}, fmt.Errorf("cannot parse repo %q: expected owner/repo, a clone URL, or a GitHub /tree/ link", arg)
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 || segs[0] == "" || segs[1] == "" {
		return Ref{}, fmt.Errorf("%q has no owner/repo in its path", arg)
	}
	ref := Ref{Name: repoName(segs[1])}
	if len(segs) >= 4 && segs[2] == "tree" {
		ref.Branch = segs[3]
		ref.Subdir = strings.Join(segs[4:], "/")
	}
	u.Path = "/" + segs[0] + "/" + segs[1]
	u.RawQuery, u.Fragment = "", ""
	ref.URL = u.String()
	return ref, nil
}

func repoName(s string) string {
	s = strings.TrimSuffix(s, ".git")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// Clone makes a shallow clone of ref into a fresh temp directory and returns
// it along with its cleanup func.
func Clone(ref Ref) (string, func(), error) {
	dir, err := os.MkdirTemp("", "memex-clone-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	args := []string{"clone", "--depth", "1"}
	if ref.Branch != "" {
		args = append(args, "--branch", ref.Branch)
	}
	args = append(args, "--", ref.URL, dir)
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("git clone %s failed:\n%s", ref.URL, strings.TrimSpace(stderr.String()))
	}
	return dir, cleanup, nil
}

// Skill is one skill found in a clone.
type Skill struct {
	Name        string
	Path        string // absolute path inside the clone
	Rel         string // repo-relative dir, for display
	Description string // frontmatter description, may be empty
}

// Discover returns every directory under the clone (restricted to ref.Subdir
// when set) that holds a SKILL.md. Dot-directories are skipped and found
// skills are not descended into. A skill at the repo root takes ref.Name.
func Discover(root string, ref Ref) ([]Skill, error) {
	base := root
	if ref.Subdir != "" {
		base = filepath.Join(root, filepath.FromSlash(ref.Subdir))
		if fi, err := os.Stat(base); err != nil || !fi.IsDir() {
			return nil, fmt.Errorf("repo has no directory %q on this branch", ref.Subdir)
		}
	}
	var skills []Skill
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != base && d.Name()[0] == '.' {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.Base(path)
		if rel == "." {
			name = ref.Name
		}
		skills = append(skills, Skill{
			Name:        name,
			Path:        path,
			Rel:         filepath.ToSlash(rel),
			Description: description(path),
		})
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Rel < skills[j].Rel })
	return skills, nil
}

// description extracts the frontmatter description from dir/SKILL.md. Block
// scalars (>, |) are folded onto one line; missing frontmatter yields "".
func description(dir string) string {
	f, err := os.Open(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 64*1024)
	inFront := false
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.TrimSpace(line) == "---":
			if inFront {
				return ""
			}
			inFront = true
		case inFront && strings.HasPrefix(line, "description:"):
			v := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			if v == "" || v[0] == '>' || v[0] == '|' {
				return foldBlock(sc)
			}
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

// foldBlock joins the indented lines of a YAML block scalar into one line.
func foldBlock(sc *bufio.Scanner) string {
	var parts []string
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") {
			break
		}
		parts = append(parts, strings.TrimSpace(line))
	}
	return strings.Join(parts, " ")
}

// Copy copies skill s into libraryDir under its name, refusing to overwrite
// an existing entry.
func Copy(s Skill, libraryDir string) error {
	dest := filepath.Join(libraryDir, s.Name)
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("library already has a skill named %q", s.Name)
	}
	return os.CopyFS(dest, os.DirFS(s.Path))
}

// Update replaces libraryDir/s.Name with the clone's version of s. The dir is
// recreated at the same path, so target symlinks pointing at it stay valid.
func Update(s Skill, libraryDir string) error {
	dest := filepath.Join(libraryDir, s.Name)
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return os.CopyFS(dest, os.DirFS(s.Path))
}
