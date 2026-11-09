package ghrepo

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-github/v80/github"
	"github.com/spf13/afero"
)

func TestLatestReleaseVersion(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		want    string
		wantErr string
	}{
		{"tag", respond(http.StatusOK, map[string]any{"tag_name": "v1.2.3"}), "1.2.3", ""},
		{"not semver", respond(http.StatusOK, map[string]any{"tag_name": "latest"}), "", "invalid semantic version"},
		{"no release", respond(http.StatusNotFound, map[string]any{"message": "Not Found"}), "", "getting latest release"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s, _ := newTestService(t, map[string]http.HandlerFunc{"GET /repos/o/n/releases/latest": tc.handler})
			r := &Repository{owner: "o", name: "n", s: s}

			v, err := r.LatestReleaseVersion(t.Context())
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}

				return
			}

			if err != nil || v.String() != tc.want {
				t.Fatalf("LatestReleaseVersion() = %v, %v; want %s", v, err, tc.want)
			}
		})
	}
}

func TestCreateRelease(t *testing.T) {
	t.Parallel()

	s, rec := newTestService(t, map[string]http.HandlerFunc{
		"POST /repos/o/n/releases": respond(http.StatusCreated, map[string]any{"id": 42, "tag_name": "v2.0.0"}),
	})
	r := &Repository{owner: "o", name: "n", s: s}

	rel, err := r.CreateRelease(t.Context(), &github.RepositoryRelease{TagName: new("v2.0.0"), Draft: new(true)})
	if err != nil {
		t.Fatal(err)
	}

	if rel.GetID() != 42 {
		t.Fatalf("ID = %d, want 42", rel.GetID())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.requests()[0].Body, &body); err != nil {
		t.Fatal(err)
	}

	if body["tag_name"] != "v2.0.0" || body["draft"] != true {
		t.Fatalf("body = %v", body)
	}
}

func TestUploadReleaseBinary(t *testing.T) {
	t.Parallel()

	const content = "binary content binary content binary content"

	setup := func(t *testing.T, h http.HandlerFunc) (*Repository, os.FileInfo, *recorder) {
		t.Helper()

		s, rec := newTestService(t, map[string]http.HandlerFunc{"POST /repos/o/n/releases/7/assets": h})
		dir := t.TempDir()
		r := &Repository{Fs: afero.NewBasePathFs(afero.NewOsFs(), dir), owner: "o", name: "n", s: s}

		if err := os.WriteFile(filepath.Join(dir, "tool"), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}

		info, err := os.Stat(filepath.Join(dir, "tool"))
		if err != nil {
			t.Fatal(err)
		}

		return r, info, rec
	}

	t.Run("zip and checksum", func(t *testing.T) {
		t.Parallel()

		r, info, rec := setup(t, respond(http.StatusCreated, map[string]any{"id": 1}))
		if err := r.UploadReleaseBinary(t.Context(), 7, "tool", info, ".exe"); err != nil {
			t.Fatal(err)
		}

		reqs := rec.requests()
		if len(reqs) != 2 {
			t.Fatalf("got %d uploads, want 2", len(reqs))
		}

		for i, want := range []string{"tool.zip", "tool_checksum_sha256.txt"} {
			q, _ := url.ParseQuery(reqs[i].Query)
			if q.Get("name") != want {
				t.Fatalf("upload %d name = %q, want %q", i, q.Get("name"), want)
			}

			if reqs[i].ContentLength != int64(len(reqs[i].Body)) {
				t.Fatalf("upload %d Content-Length = %d, body has %d bytes", i, reqs[i].ContentLength, len(reqs[i].Body))
			}
		}

		zr, err := zip.NewReader(bytes.NewReader(reqs[0].Body), int64(len(reqs[0].Body)))
		if err != nil {
			t.Fatal(err)
		}

		if len(zr.File) != 1 || zr.File[0].Name != "n.exe" || zr.File[0].Method != zip.Deflate {
			t.Fatalf("zip entries = %v, want one deflated n.exe", zr.File)
		}

		f, err := zr.File[0].Open()
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		if b, _ := io.ReadAll(f); string(b) != content {
			t.Fatalf("zipped content = %q", b)
		}

		sum := sha256.Sum256(reqs[0].Body)
		if got := string(reqs[1].Body); got != hex.EncodeToString(sum[:]) {
			t.Fatalf("checksum = %q, want the SHA-256 of the uploaded zip", got)
		}
	})

	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{"zip upload fails", respond(http.StatusInternalServerError, map[string]any{"message": "boom"}), `uploading "tool.zip"`},
		{"unexpected status", respond(http.StatusOK, map[string]any{"id": 1}), "upload failed with status 200 OK"},
		{"checksum upload fails", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Query().Get("name"), ".txt") {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "boom"})
				return
			}

			writeJSON(w, http.StatusCreated, map[string]any{"id": 1})
		}, `uploading "tool_checksum_sha256.txt"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, info, _ := setup(t, tc.handler)

			err := r.UploadReleaseBinary(t.Context(), 7, "tool", info, "")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		r, info, rec := setup(t, respond(http.StatusCreated, map[string]any{"id": 1}))

		err := r.UploadReleaseBinary(t.Context(), 7, "nope", info, "")
		if err == nil || !strings.Contains(err.Error(), "opening file") || len(rec.requests()) != 0 {
			t.Fatalf("err = %v with %d uploads", err, len(rec.requests()))
		}
	})
}

// TestUploadReleaseBinary_Quiet is not parallel because it swaps the log output.
func TestUploadReleaseBinary_Quiet(t *testing.T) {
	var logs bytes.Buffer

	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	s, _ := newTestService(t, map[string]http.HandlerFunc{
		"POST /repos/o/n/releases/1/assets": respond(http.StatusCreated, map[string]any{"id": 1}),
	})
	dir := t.TempDir()
	r := &Repository{Fs: afero.NewBasePathFs(afero.NewOsFs(), dir), owner: "o", name: "n", s: s}

	if err := os.WriteFile(filepath.Join(dir, "tool"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(dir, "tool"))
	if err != nil {
		t.Fatal(err)
	}

	if err := r.UploadReleaseBinary(t.Context(), 1, "tool", info, ""); err != nil {
		t.Fatal(err)
	}

	if logs.Len() != 0 {
		t.Fatalf("a successful upload logged %q", logs.String())
	}
}
