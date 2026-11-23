package storage

import (
	"context"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const ReceiptDirectory = "receipts"

var receiptName = regexp.MustCompile(`^[a-f0-9]{32}\.podreceipt$`)

func ValidReceiptName(name string) bool { return receiptName.MatchString(name) }

// Receipts are not readable through the ordinary asset store.
type Receipts struct{ Root string }

func (s Receipts) local() Local { return Local{Path: filepath.Join(s.Root, ReceiptDirectory)} }

func (s Receipts) Put(ctx context.Context, name string, data []byte) error {
	return s.local().putVerified(ctx, name, data, receiptName)
}

func (s Receipts) Open(ctx context.Context, name string) (*File, io.ReaderAt, error) {
	f, err := s.local().open(ctx, name, receiptName)
	if err != nil {
		return nil, nil, err
	}
	return f, f.ReadCloser.(io.ReaderAt), nil // Local.open returns an *os.File.
}

func (s Receipts) Remove(ctx context.Context, name string) error {
	return s.local().remove(ctx, name, receiptName)
}

func (s Receipts) Check(ctx context.Context) error {
	name := strings.ReplaceAll(uuid.NewString(), "-", "") + ".podreceipt"
	if err := s.Put(ctx, name, []byte("probe")); err != nil {
		return err
	}
	return s.Remove(ctx, name)
}
