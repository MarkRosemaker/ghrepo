package ghrepo

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v80/github"
)

// lenChecksum is the length of a SHA-256 checksum when encoded as hexadecimal (64 characters).
const lenChecksum int64 = 64

// UploadReleaseBinary zips a binary file and uploads it as a release asset to a GitHub release.
// It also computes a SHA-256 checksum during the upload and uploads a separate checksum file.
//
// The binary is placed inside a zip archive with a single entry. The name of the file inside the zip
// is the repository name with an optional suffix (e.g., ".exe" for Windows binaries).
func (r *Repository) UploadReleaseBinary(ctx context.Context, relID int,
	path string, info fs.FileInfo, suffix string) error {
	src, err := r.Open(path)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer src.Close()

	// We need to zip the binary locally and then upload because we need to know its size.
	tmpPath, err := r.zipBinary(src, info, suffix)
	if err != nil {
		return fmt.Errorf("zipping binary: %w", err)
	}
	defer os.Remove(tmpPath)

	fi, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	defer fi.Close()

	stat, err := fi.Stat()
	if err != nil {
		return err
	}

	// Now that we have the size, upload the zipped binary while getting its hash.
	hash := sha256.New()
	zipName := info.Name() + ".zip"
	if _, err := r.uploadReleaseAsset(ctx, relID, zipName, io.TeeReader(fi, hash), stat.Size()); err != nil {
		return fmt.Errorf("uploading %q: %w", zipName, err)
	}

	// Now that we have the hash, upload the checksum file.
	checksumName := fmt.Sprintf("%s_checksum_sha256.txt", info.Name())
	if _, err := r.uploadReleaseAsset(ctx, relID, checksumName,
		strings.NewReader(hex.EncodeToString(hash.Sum(nil))),
		lenChecksum); err != nil {
		return fmt.Errorf("uploading %q: %w", checksumName, err)
	}

	return nil
}

// zipBinary creates a temporary zip file containing a single binary entry.
// Returns the path to the temporary zip file.
func (r *Repository) zipBinary(fi io.Reader, info fs.FileInfo, suffix string) (string, error) {
	tmp, err := os.CreateTemp("", r.name+".zip")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	fh, err := zip.FileInfoHeader(info)
	if err != nil {
		return "", err
	}

	// Using FileInfoHeader() above uses the basename of the file.
	// If we want to name the file inside the zip differently, we need to overwrite it.
	fh.Name = r.name + suffix

	// Change to deflate to gain better compression
	// see http://golang.org/pkg/archive/zip/#pkg-constants
	fh.Method = zip.Deflate

	zw := zip.NewWriter(tmp)
	defer zw.Close()

	h, err := zw.CreateHeader(fh)
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(h, fi); err != nil {
		return "", err
	}

	return tmp.Name(), nil
}

// uploadReleaseAsset uploads a single release asset to GitHub.
//
// It constructs the upload URL, sets the correct Content-Type based on file extension,
// and performs the HTTP request using the go-github client.
//
// Returns the created ReleaseAsset on success.
func (r *Repository) uploadReleaseAsset(ctx context.Context, relID int,
	assetName string, reader io.Reader, size int64) (*github.ReleaseAsset, error) {
	req, err := r.s.github.NewUploadRequest(
		fmt.Sprintf("repos/%s/%s/releases/%d/assets?name=%s", r.owner, r.name, relID, assetName),
		reader, size,
		mime.TypeByExtension(filepath.Ext(assetName)))
	if err != nil {
		return nil, err
	}

	asset := &github.ReleaseAsset{}
	resp, err := r.s.github.Do(ctx, req, asset)
	if err != nil {
		return nil, err
	}

	// Check for successful creation
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("got status code %d %s: %s",
			resp.StatusCode, http.StatusText(resp.StatusCode), string(b))
	}

	return asset, nil
}
