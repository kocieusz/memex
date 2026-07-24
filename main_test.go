package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kocieusz/memex/internal/library"
	"github.com/kocieusz/memex/internal/origin"
	"github.com/kocieusz/memex/internal/remote"
)

func mkSkill(t *testing.T, root, name, content string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func libHash(t *testing.T, dir string) string {
	t.Helper()
	h, err := origin.HashDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestClassifyClone(t *testing.T) {
	repoURL := "https://github.com/a/b"
	clone := t.TempDir()
	lib := t.TempDir()

	newDir := mkSkill(t, clone, "new-skill", "new")     // only in the clone
	sameDir := mkSkill(t, clone, "same-skill", "same")  // tracked, upstream unchanged
	mkSkill(t, lib, "same-skill", "same")               //
	chgDir := mkSkill(t, clone, "changed-skill", "v2")  // tracked, upstream changed
	chgLib := mkSkill(t, lib, "changed-skill", "v1")    //
	editDir := mkSkill(t, clone, "edited-skill", "v2")  // tracked, upstream changed + local edits
	mkSkill(t, lib, "edited-skill", "v1 plus my edits") //
	dupDir := mkSkill(t, clone, "native-skill", "x")    // in the library without an origin
	mkSkill(t, lib, "native-skill", "y")                //

	origins := map[string]origin.Origin{
		"same-skill":    {Repo: repoURL, Path: "same-skill", Hash: libHash(t, sameDir)},
		"changed-skill": {Repo: repoURL, Path: "changed-skill", Hash: libHash(t, chgLib)},
		"edited-skill":  {Repo: repoURL, Path: "edited-skill", Hash: "hash-at-clone-time"},
	}
	found := []remote.Skill{
		{Name: "changed-skill", Path: chgDir, Rel: "changed-skill"},
		{Name: "edited-skill", Path: editDir, Rel: "edited-skill"},
		{Name: "native-skill", Path: dupDir, Rel: "native-skill"},
		{Name: "new-skill", Path: newDir, Rel: "new-skill"},
		{Name: "same-skill", Path: sameDir, Rel: "same-skill"},
	}
	skills, err := library.Scan(lib)
	if err != nil {
		t.Fatal(err)
	}

	items, updates, hashes, err := classifyClone(found, skills, lib, origins, repoURL)
	if err != nil {
		t.Fatal(err)
	}

	if items[0].Conflict != "" || !updates[0] || !strings.Contains(items[0].Note, "update available") {
		t.Errorf("changed-skill: want selectable update, got %+v updates=%v", items[0], updates[0])
	}
	if want := libHash(t, chgDir); hashes[0] != want {
		t.Errorf("changed-skill hash = %q, want upstream hash %q", hashes[0], want)
	}
	if !updates[1] || !strings.Contains(items[1].Note, "local edits") {
		t.Errorf("edited-skill: want overwrite warning, got %+v", items[1])
	}
	if items[2].Conflict != "already in the library" {
		t.Errorf("native-skill: want library conflict, got %+v", items[2])
	}
	if items[3].Conflict != "" || updates[3] {
		t.Errorf("new-skill: want plain selectable, got %+v updates=%v", items[3], updates[3])
	}
	if items[4].Conflict != "up to date" {
		t.Errorf("same-skill: want up to date, got %+v", items[4])
	}
}

func TestClassifyCloneOtherRepo(t *testing.T) {
	clone := t.TempDir()
	lib := t.TempDir()
	dir := mkSkill(t, clone, "skill-a", "upstream")
	mkSkill(t, lib, "skill-a", "mine")
	origins := map[string]origin.Origin{
		"skill-a": {Repo: "https://github.com/other/repo", Path: "skill-a", Hash: "whatever"},
	}
	skills, err := library.Scan(lib)
	if err != nil {
		t.Fatal(err)
	}
	items, updates, _, err := classifyClone([]remote.Skill{{Name: "skill-a", Path: dir, Rel: "skill-a"}}, skills, lib, origins, "https://github.com/a/b")
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Conflict != "already in the library" || updates[0] {
		t.Errorf("skill from another repo must not be offered as update, got %+v", items[0])
	}
}
