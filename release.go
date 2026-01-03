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

const lenChecksum int64 = 64

// UploadReleaseBinary zips and uploads a binary to a release, along with its checksum.
// The file name inside the zip is called the same as the repository name, with optional suffix. For windows binaries, set suffix to ".exe".
func (r *Repository) UploadReleaseBinary(ctx context.Context, relID int,
	path string, info fs.FileInfo, suffix string) error {
	src, err := r.Open(path)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer src.Close()

	// We need to zip the binary locally and then upload because we need to know its size.
	tmpPath, err := r.zipBinary(filepath.Base(path), src, info, suffix)
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

func (r *Repository) zipBinary(fullName string, fi io.Reader, info fs.FileInfo, suffix string) (string, error) {
	tmpZip, err := os.CreateTemp("", fullName+".zip")
	if err != nil {
		return "", err
	}
	defer tmpZip.Close()

	zipHeader, err := zip.FileInfoHeader(info)
	if err != nil {
		return "", err
	}

	// Using FileInfoHeader() above uses the basename of the file.
	// If we want to name the file inside the zip differently, we need to overwrite it.
	zipHeader.Name = r.name + suffix

	// Change to deflate to gain better compression
	// see http://golang.org/pkg/archive/zip/#pkg-constants
	zipHeader.Method = zip.Deflate

	zipWriter := zip.NewWriter(tmpZip)
	defer zipWriter.Close()

	h, err := zipWriter.CreateHeader(zipHeader)
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(h, fi); err != nil {
		return "", err
	}

	return tmpZip.Name(), nil
}

// uploadReleaseAsset uploads a release asset to the specified release ID.
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

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("got status code %d %s: %s",
			resp.StatusCode, http.StatusText(resp.StatusCode), string(b))
	}

	return asset, nil
}
