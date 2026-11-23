package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/assetimage"
	"ruoyi-go/pkg/executor"
	"ruoyi-go/pkg/storage"
)

const localReceiptRef = "pod-local:"
const maxReceiptManifest = 256 << 10
const maxReceiptFile = 49 << 20

func (s *PODAssetService) receiptStore() (storage.Receipts, error) {
	local, ok := s.Store.(storage.Local)
	if !ok || local.Path == "" {
		return storage.Receipts{}, errors.New("private receipt storage unavailable")
	}
	return storage.Receipts{Root: local.Path}, nil
}

func receiptFileName(submission string) string {
	hash := sha256.Sum256([]byte("pod-managed-image\x00" + submission))
	return hex.EncodeToString(hash[:16]) + ".podreceipt"
}

func writeReceiptEntry(w *zip.Writer, name string, data []byte) error {
	entry, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	if err != nil {
		return err
	}
	_, err = entry.Write(data)
	return err
}

func stagePODInlineReceipt(ctx context.Context, store storage.Receipts, submission string, receipt executor.QwenReceipt, limits model.PODAssetLimits) (executor.QwenReceipt, error) {
	if receipt.ImageSource != "catalog-v1" || !hasPODImageRef(receipt.ImageURLs, "data:") {
		return receipt, nil
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	refs := make([]string, 0, len(receipt.ImageURLs))
	localCount := 0
	for _, raw := range receipt.ImageURLs {
		if !strings.HasPrefix(raw, "data:") {
			refs = append(refs, raw)
			continue
		}
		select {
		case podUploadSlots <- struct{}{}:
		case <-ctx.Done():
			return receipt, ctx.Err()
		}
		data, err := executor.DownloadCatalogImage(ctx, nil, raw, limits.MaxUploadBytes)
		if err == nil {
			media := http.DetectContentType(data)
			ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[media]
			_, err = assetimage.Prepare(ctx, bytes.NewReader(data), "image."+ext, media, assetimage.Limits{MaxBytes: limits.MaxUploadBytes, MaxPixels: limits.MaxPixels, MaxDimension: limits.MaxDimension})
		}
		<-podUploadSlots
		if ctx.Err() != nil {
			return receipt, ctx.Err()
		}
		if err != nil {
			receipt.State, receipt.Code, receipt.Message = "failed", "invalidProviderImage", "供应商返回的部分或全部图片不合法或超过限制；有效图片保留，不会自动重发"
			continue
		}
		ref := localReceiptRef + strconv.Itoa(len(refs))
		if len(data) > maxReceiptFile-buffer.Len()-maxReceiptManifest {
			return receipt, errors.New("private image receipt exceeds storage bound")
		}
		if err := writeReceiptEntry(archive, receiptImageEntry(len(refs)), data); err != nil {
			return receipt, err
		}
		refs = append(refs, ref)
		localCount++
	}
	receipt.ImageURLs = refs
	if localCount == 0 {
		return receipt, nil
	}
	manifest, err := json.Marshal(receipt)
	if err != nil || len(manifest) > maxReceiptManifest {
		return receipt, errors.New("invalid private image receipt manifest")
	}
	if err := writeReceiptEntry(archive, "receipt.json", manifest); err != nil {
		return receipt, err
	}
	if err := archive.Close(); err != nil {
		return receipt, err
	}
	return receipt, store.Put(ctx, receiptFileName(submission), buffer.Bytes())
}

func hasPODImageRef(refs []string, prefix string) bool {
	for _, ref := range refs {
		if strings.HasPrefix(ref, prefix) {
			return true
		}
	}
	return false
}

func receiptImageEntry(index int) string { return "image-" + strconv.Itoa(index) }

func readPODReceiptEntry(ctx context.Context, store storage.Receipts, submission, entry string, limit int64) ([]byte, error) {
	f, reader, err := store.Open(ctx, receiptFileName(submission))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if f.Size > maxReceiptFile {
		return nil, errors.New("private image receipt exceeds storage bound")
	}
	archive, err := zip.NewReader(reader, f.Size)
	if err != nil || len(archive.File) > 17 {
		return nil, errors.New("invalid private image receipt")
	}
	for _, part := range archive.File {
		if part.Name != entry {
			continue
		}
		if part.Method != zip.Store || part.UncompressedSize64 > uint64(limit) {
			break
		}
		file, err := part.Open()
		if err != nil {
			return nil, err
		}
		defer file.Close()
		var data bytes.Buffer
		n, err := storage.Copy(ctx, &data, io.LimitReader(file, limit+1))
		if err != nil || n > limit || n != int64(part.UncompressedSize64) {
			return nil, errors.New("private image receipt entry is invalid")
		}
		return data.Bytes(), nil
	}
	return nil, errors.New("private image receipt entry unavailable")
}

func loadPODInlineReceipt(ctx context.Context, store storage.Receipts, submission string) (executor.QwenReceipt, error) {
	var receipt executor.QwenReceipt
	data, err := readPODReceiptEntry(ctx, store, submission, "receipt.json", maxReceiptManifest)
	if err != nil {
		return receipt, err
	}
	if json.Unmarshal(data, &receipt) != nil || receipt.ImageSource != "catalog-v1" || (receipt.State != "succeeded" && receipt.State != "failed") || !hasPODImageRef(receipt.ImageURLs, localReceiptRef) || hasPODImageRef(receipt.ImageURLs, "data:") {
		return receipt, errors.New("invalid private image receipt metadata")
	}
	return receipt, nil
}
