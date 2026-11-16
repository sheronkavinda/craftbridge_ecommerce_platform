
package catalog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ImageStore interface {
	Save(ctx context.Context, data []byte, extension string) (string, error)
	Delete(ctx context.Context, imageURL string) error
}

type LocalImageStore struct {
	directory string
}

func NewLocalImageStore(directory string) (*LocalImageStore, error) {
	if err := os.MkdirAll(directory, 0750); err != nil {
		return nil, err
	}

	return &LocalImageStore{
		directory: directory,
	}, nil
}

// Save creates a random filename and stores image bytes.
func (s *LocalImageStore) Save(
	ctx context.Context,
	data []byte,
	extension string,
) (string, error) {

	if err := ctx.Err(); err != nil {
		return "", err
	}

	switch extension {
	case ".jpg", ".png", ".webp":
	default:
		return "", errors.New("unsupported image extension")
	}

	randomBytes := make([]byte, 16)

	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	filename := hex.EncodeToString(randomBytes) + extension
	fullPath := filepath.Join(s.directory, filename)

	file, err := os.OpenFile(
		fullPath,
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0640,
	)
	if err != nil {
		return "", err
	}

	_, writeErr := file.Write(data)
	closeErr := file.Close()

	if writeErr != nil || closeErr != nil {
		os.Remove(fullPath)

		return "", fmt.Errorf(
			"image write failed: %v %v",
			writeErr, closeErr,
		)
	}

	return "/media/catalog/" + filename, nil
}

// Delete removes an image using only its generated filename.
func (s *LocalImageStore) Delete(
	ctx context.Context,
	imageURL string,
) error {

	if err := ctx.Err(); err != nil {
		return err
	}

	const prefix = "/media/catalog/"

	if !strings.HasPrefix(imageURL, prefix) {
		return errors.New("invalid local image path")
	}

	filename := strings.TrimPrefix(imageURL, prefix)

	if filename == "" ||
		filename == "." ||
		filepath.Base(filename) != filename ||
		strings.ContainsAny(filename, `/\`) {
		return errors.New("unsafe image filename")
	}

	fullPath := filepath.Join(s.directory, filename)

	err := os.Remove(fullPath)

	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}
