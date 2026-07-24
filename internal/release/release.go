// Package release finds published memex builds on GitHub and swaps the
// running binary for a newer one.
package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// Repo is the upstream this binary upgrades from.
const Repo = "kocieusz/memex"

// maxAsset caps what a single download may expand to — a guard against a
// truncated or hostile response, not a real size limit; archives are ~2 MB.
const maxAsset = 64 << 20

// Source locates releases. The zero value is unusable; call GitHub.
type Source struct {
	API      string // URL returning the latest release as JSON
	Download string // base URL that release assets hang off
	Client   *http.Client
}

// GitHub points at the upstream repo's releases.
func GitHub() *Source {
	return &Source{
		API:      "https://api.github.com/repos/" + Repo + "/releases/latest",
		Download: "https://github.com/" + Repo + "/releases/download",
	}
}

func (s *Source) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return http.DefaultClient
}

// Latest reports the tag of the newest published release.
func (s *Source) Latest(ctx context.Context) (string, error) {
	body, err := s.get(ctx, s.API)
	if err != nil {
		return "", fmt.Errorf("checking for the latest release: %w", err)
	}
	var rel struct {
		Tag string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", fmt.Errorf("parsing the release response: %w", err)
	}
	if rel.Tag == "" {
		return "", fmt.Errorf("no published release found for %s", Repo)
	}
	return rel.Tag, nil
}

// AssetName is the archive published for an OS/arch pair. It mirrors the
// archive name_template in .goreleaser.yaml — the two must stay in step or
// installed binaries can't find their upgrade.
func AssetName(tag, goos, goarch string) string {
	return fmt.Sprintf("memex_%s_%s_%s.tar.gz", strings.TrimPrefix(tag, "v"), goos, goarch)
}

// Binary downloads the release archive for an OS/arch pair, checks it against
// the release's checksums.txt, and returns the memex executable inside it.
func (s *Source) Binary(ctx context.Context, tag, goos, goarch string) ([]byte, error) {
	name := AssetName(tag, goos, goarch)
	base := s.Download + "/" + tag + "/"

	archive, err := s.get(ctx, base+name)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", name, err)
	}
	sums, err := s.get(ctx, base+"checksums.txt")
	if err != nil {
		return nil, fmt.Errorf("downloading checksums for %s: %w", tag, err)
	}
	want, err := checksum(string(sums), name)
	if err != nil {
		return nil, err
	}
	if got := sha256.Sum256(archive); hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("checksum mismatch for %s — the download was corrupted or tampered with", name)
	}
	return extract(archive)
}

func (s *Source) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "memex")
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxAsset))
}

// checksum pulls the expected hash for name out of a goreleaser checksums.txt
// ("<sha256>  <file>" per line).
func checksum(sums, name string) (string, error) {
	for line := range strings.SplitSeq(sums, "\n") {
		hash, file, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok && strings.TrimSpace(file) == name {
			return hash, nil
		}
	}
	return "", fmt.Errorf("%s is missing from checksums.txt", name)
}

// extract returns the memex executable from a .tar.gz archive.
func extract(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("reading the downloaded archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading the downloaded archive: %w", err)
		}
		if h.Typeflag != tar.TypeReg || path.Base(h.Name) != "memex" {
			continue
		}
		return io.ReadAll(io.LimitReader(tr, maxAsset))
	}
	return nil, fmt.Errorf("the downloaded archive contains no memex binary")
}

// Replace swaps the file at path for bin. The new binary is staged in the same
// directory so the final rename is atomic and leaves no copy behind — an
// interrupted upgrade leaves the old binary working.
func Replace(path string, bin []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".memex-*")
	if err != nil {
		return fmt.Errorf("%s is not writable — reinstall with the install script, or re-run as a user who owns it: %w", dir, err)
	}
	staged := tmp.Name()
	defer os.Remove(staged) // no-op once the rename succeeds

	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		return err
	}
	return os.Rename(staged, path)
}

// Newer reports whether tag is a later version than current. A current version
// that isn't a release tag — a local `go build`, which reports (devel) —
// counts as older, so upgrading from a dev build is always offered.
func Newer(current, tag string) bool {
	cur, ok := parse(current)
	if !ok {
		return true
	}
	next, ok := parse(tag)
	if !ok {
		return false
	}
	for i := range 3 {
		if next[i] != cur[i] {
			return next[i] > cur[i]
		}
	}
	return false
}

// parse reads a vMAJOR.MINOR.PATCH tag, ignoring any pre-release suffix.
func parse(tag string) ([3]int, bool) {
	var v [3]int
	s := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	s, _, _ = strings.Cut(s, "-") // drop any -rc1 pre-release suffix
	s, _, _ = strings.Cut(s, "+") // drop any +build metadata
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}
