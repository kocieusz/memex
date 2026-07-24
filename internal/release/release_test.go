package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		current, tag string
		want         bool
	}{
		{"v0.3.0", "v0.4.0", true},
		{"v0.3.0", "v0.3.1", true},
		{"v0.3.0", "v1.0.0", true},
		{"v0.3.0", "v0.3.0", false},
		{"v0.4.0", "v0.3.9", false},
		{"0.3.0", "v0.4.0", true},      // missing v prefix
		{"v0.3.0", "v0.4.0-rc1", true}, // pre-release suffix ignored
		{"(devel)", "v0.1.0", true},    // dev build always upgrades
		{"unknown", "v0.1.0", true},
		{"v0.3.0", "garbage", false}, // unparseable tag is never newer
	}
	for _, c := range cases {
		if got := Newer(c.current, c.tag); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.current, c.tag, got, c.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	if got := AssetName("v0.4.0", "darwin", "arm64"); got != "memex_0.4.0_darwin_arm64.tar.gz" {
		t.Errorf("AssetName = %q", got)
	}
}

// tarGz packs a single file into a .tar.gz, as goreleaser's archive step does.
func tarGz(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// releaseServer serves an archive and matching checksums.txt for one tag.
func releaseServer(t *testing.T, tag, asset string, archive []byte) *Source {
	t.Helper()
	sum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), asset)
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q}`, tag)
	})
	mux.HandleFunc("/dl/"+tag+"/"+asset, func(w http.ResponseWriter, _ *http.Request) {
		w.Write(archive)
	})
	mux.HandleFunc("/dl/"+tag+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, checksums)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &Source{API: srv.URL + "/latest", Download: srv.URL + "/dl", Client: srv.Client()}
}

func TestLatest(t *testing.T) {
	src := releaseServer(t, "v0.4.0", "x", nil)
	got, err := src.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "v0.4.0" {
		t.Errorf("Latest = %q, want v0.4.0", got)
	}
}

func TestBinary(t *testing.T) {
	tag := "v0.4.0"
	asset := AssetName(tag, "darwin", "arm64")
	archive := tarGz(t, "memex", []byte("i am the binary"))
	src := releaseServer(t, tag, asset, archive)

	bin, err := src.Binary(context.Background(), tag, "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if string(bin) != "i am the binary" {
		t.Errorf("Binary = %q", bin)
	}
}

func TestBinaryChecksumMismatch(t *testing.T) {
	tag := "v0.4.0"
	asset := AssetName(tag, "linux", "amd64")
	// The checksums.txt is computed for one archive but a different one is
	// served, mimicking a corrupted or tampered download.
	claimed := tarGz(t, "memex", []byte("what the checksum promises"))
	served := tarGz(t, "memex", []byte("what actually arrives"))
	sum := sha256.Sum256(claimed)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), asset)

	mux := http.NewServeMux()
	mux.HandleFunc("/dl/"+tag+"/"+asset, func(w http.ResponseWriter, _ *http.Request) { w.Write(served) })
	mux.HandleFunc("/dl/"+tag+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, checksums) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	src := &Source{Download: srv.URL + "/dl", Client: srv.Client()}

	if _, err := src.Binary(context.Background(), tag, "linux", "amd64"); err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
}

func TestReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memex")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Replace(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("after Replace, binary = %q, want new", got)
	}
	// The staging temp file must not linger next to the binary.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected only the binary in %s, got %d entries", dir, len(entries))
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("replaced binary is not executable: %v", fi.Mode())
	}
}
