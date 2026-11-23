// Package storage provides private, root-confined file storage, without HTTP or ownership decisions.
package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"regexp"
)

type Store interface {
	Put(context.Context, string, []byte) (string, error)
	Open(context.Context, string) (*File, error)
	Remove(context.Context, string) error
}

type File struct {
	io.ReadCloser
	Size int64
}

type Local struct{ Path string }

// PutVerified resumes a persisted copy plan at the same file key. A completed
// file must match exactly; partial writes remain temporary and are never read.
func (s Local) PutVerified(ctx context.Context, key string, data []byte) error {
	return s.putVerified(ctx, key, data, keyPattern)
}

func (s Local) putVerified(ctx context.Context, key string, data []byte, pattern *regexp.Regexp) error {
	if !pattern.MatchString(key) {
		return errors.New("invalid storage key")
	}
	if f, err := s.open(ctx, key, pattern); err == nil {
		defer f.Close()
		old, err := io.ReadAll(io.LimitReader(f, int64(len(data))+1))
		if err != nil {
			return err
		}
		if !bytes.Equal(old, data) {
			return errors.New("existing copy differs from plan")
		}
		return ctx.Err()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(s.Path, 0700); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.Path)
	if err != nil {
		return err
	}
	defer root.Close()
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	temp := hex.EncodeToString(random) + ".partial"
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { f.Close(); _ = root.Remove(temp) }()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// Link publishes a complete file without replacing an existing destination.
	// Rename would overwrite a concurrent request with different bytes on Unix.
	if err := root.Link(temp, key); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		return s.putVerified(ctx, key, data, pattern)
	}
	return nil
}

var keyPattern = regexp.MustCompile(`^[a-f0-9]{32}\.(png|jpg|webp)$`)

func (s Local) Put(ctx context.Context, extension string, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if extension != "png" && extension != "jpg" && extension != "webp" {
		return "", errors.New("invalid storage extension")
	}
	if err := os.MkdirAll(s.Path, 0700); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(s.Path)
	if err != nil {
		return "", err
	}
	defer root.Close()
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	key := hex.EncodeToString(id) + "." + extension
	file, err := root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	complete := false
	defer func() {
		file.Close()
		if !complete {
			_ = root.Remove(key)
		}
	}()
	if _, err = file.Write(data); err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if err = file.Sync(); err != nil {
		return "", err
	}
	if err = file.Close(); err != nil {
		return "", err
	}
	complete = true
	return key, nil
}

func (s Local) Open(ctx context.Context, key string) (*File, error) {
	return s.open(ctx, key, keyPattern)
}

func (s Local) open(ctx context.Context, key string, pattern *regexp.Regexp) (*File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !pattern.MatchString(key) {
		return nil, errors.New("invalid storage key")
	}
	root, err := os.OpenRoot(s.Path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(key)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("invalid storage file")
	}
	return &File{ReadCloser: file, Size: info.Size()}, nil
}
func (s Local) Remove(ctx context.Context, key string) error {
	return s.remove(ctx, key, keyPattern)
}

func (s Local) remove(ctx context.Context, key string, pattern *regexp.Regexp) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !pattern.MatchString(key) {
		return errors.New("invalid storage key")
	}
	root, err := os.OpenRoot(s.Path)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(key)
}

// Copy checks cancellation between bounded reads; no disk path is exposed to callers.
func Copy(ctx context.Context, w io.Writer, r io.Reader) (int64, error) {
	var total int64
	buffer := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := r.Read(buffer)
		if n > 0 {
			written, e := w.Write(buffer[:n])
			total += int64(written)
			if e != nil {
				return total, e
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}
