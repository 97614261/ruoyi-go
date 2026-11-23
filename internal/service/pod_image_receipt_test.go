package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"testing"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/executor"
	"ruoyi-go/pkg/storage"
)

func TestPODInlineReceiptStoresBinaryAndRecoversCompactMetadata(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := storage.Receipts{Root: root}
	if err := store.Check(ctx); err != nil {
		t.Fatal(err)
	}
	var raster bytes.Buffer
	if err := png.Encode(&raster, image.NewNRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	imageURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raster.Bytes())
	brokenURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raster.Bytes()[:26])
	receipt := executor.QwenReceipt{State: "succeeded", ImageSource: "catalog-v1", ImageURLs: []string{brokenURL, "https://cdn.example.com/a.png", imageURL}}
	const submission = "fixture-submission"
	staged, err := stagePODInlineReceipt(ctx, store, submission, receipt, DefaultPODAssetLimits(model.PODAssetLimits{}))
	if err != nil || staged.State != "failed" || staged.Code != "invalidProviderImage" || len(staged.ImageURLs) != 2 || staged.ImageURLs[1] != "pod-local:1" {
		t.Fatal("valid image or URL lost when another image was invalid", err)
	}
	reopened := storage.Receipts{Root: root}
	recovered, err := loadPODInlineReceipt(ctx, reopened, submission)
	if err != nil || recovered.ImageURLs[1] != "pod-local:1" {
		t.Fatal("receipt could not be reopened", err)
	}
	encoded, _ := json.Marshal(recovered)
	if bytes.Contains(encoded, []byte("base64")) || len(encoded) > 512 {
		t.Fatal("recovered database receipt is not compact")
	}
	imageBytes, err := readPODReceiptEntry(ctx, reopened, submission, receiptImageEntry(1), 1<<20)
	if err != nil || !bytes.Equal(imageBytes, raster.Bytes()) {
		t.Fatal("staged binary differs from the original image", err)
	}
	if _, err := loadPODInlineReceipt(ctx, reopened, "different-submission"); err == nil {
		t.Fatal("staged receipt was not tied to original submission")
	}
}
