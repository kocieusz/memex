// Package origin records where library skills were cloned from — the repo,
// the path inside it, and a content hash of the skill as copied — so a later
// clone of the same repo can offer updates instead of refusing duplicates.
package origin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// fileName is the manifest kept in the library root; the dot prefix keeps it
// invisible to library scans and harnesses.
const fileName = ".origins.toml"

// Origin is one skill's provenance.
type Origin struct {
	Repo string `toml:"repo"` // clone URL the skill came from
	Path string `toml:"path"` // repo-relative dir of the skill
	Hash string `toml:"hash"` // HashDir of the skill as copied
}

// File returns the manifest path for libraryDir.
func File(libraryDir string) string {
	return filepath.Join(libraryDir, fileName)
}

// Load reads the manifest. A missing file yields an empty map.
func Load(libraryDir string) (map[string]Origin, error) {
	m := map[string]Origin{}
	_, err := toml.DecodeFile(File(libraryDir), &m)
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", File(libraryDir), err)
	}
	return m, nil
}

// Save writes the manifest with sorted keys, removing the file when origins
// is empty.
func Save(libraryDir string, origins map[string]Origin) error {
	path := File(libraryDir)
	if len(origins) == 0 {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(origins); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// HashDir returns a sha256 content hash of dir: every file's slash-relative
// path and bytes in lexical walk order. .git and .DS_Store are skipped, so
// the hash compares a clone's skill dir with a library copy of it.
func HashDir(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".DS_Store" {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		h.Write([]byte(filepath.ToSlash(rel)))
		h.Write([]byte{0})
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		h.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
