package assetimage

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
)

func TestPreparePreservesOriginalAndAlpha(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 960, 480))
	source.SetNRGBA(10, 10, color.NRGBA{R: 255, A: 100})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, source); err != nil {
		t.Fatal(err)
	}
	limits := Limits{MaxBytes: 1 << 20, MaxPixels: 1_000_000, MaxDimension: 2000}
	result, err := Prepare(context.Background(), bytes.NewReader(buffer.Bytes()), "透明.png", "image/png", limits)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Original, buffer.Bytes()) || result.ThumbWidth != 480 || result.ThumbHeight != 240 {
		t.Fatal("original bytes or thumbnail dimensions changed")
	}
	decoded, err := png.Decode(bytes.NewReader(result.Thumbnail))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, a := decoded.At(100, 100).RGBA()
	if a != 0 {
		t.Fatal("transparent background lost")
	}
}
func TestPrepareRejectsDisguisesAndResourceBombs(t *testing.T) {
	var valid bytes.Buffer
	_ = png.Encode(&valid, image.NewNRGBA(image.Rect(0, 0, 4, 4)))
	bomb := append([]byte(nil), valid.Bytes()...)
	binary.BigEndian.PutUint32(bomb[16:20], 100000)
	binary.BigEndian.PutUint32(bomb[20:24], 100000)
	binary.BigEndian.PutUint32(bomb[29:33], crc32.ChecksumIEEE(bomb[12:29]))
	limits := Limits{MaxBytes: 1024, MaxPixels: 100, MaxDimension: 100}
	for _, item := range []struct {
		name, mime string
		data       []byte
	}{
		{"bad.png", "image/png", []byte("<svg></svg>")}, {"bad.jpg", "image/jpeg", valid.Bytes()}, {"bad.png", "image/jpeg", valid.Bytes()},
		{"../bad.png", "image/png", valid.Bytes()}, {`..\bad.png`, "image/png", valid.Bytes()}, {"bad.png", "image/png", valid.Bytes()[:35]},
		{"large.png", "image/png", bomb}, {"large.png", "image/png", make([]byte, 1025)},
	} {
		t.Run(item.name+item.mime, func(t *testing.T) {
			if _, err := Prepare(context.Background(), bytes.NewReader(item.data), item.name, item.mime, limits); err == nil {
				t.Fatal("unsafe image accepted")
			}
		})
	}
}
func TestJPEGAndCancellation(t *testing.T) {
	var buffer bytes.Buffer
	_ = jpeg.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 8, 4)), nil)
	limits := Limits{MaxBytes: 4096, MaxPixels: 100, MaxDimension: 100}
	result, err := Prepare(context.Background(), bytes.NewReader(buffer.Bytes()), "photo.JPEG", "application/octet-stream", limits)
	if err != nil || result.MediaType != "image/jpeg" {
		t.Fatal("JPEG not accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Prepare(ctx, bytes.NewReader(buffer.Bytes()), "photo.jpg", "image/jpeg", limits); err == nil {
		t.Fatal("canceled request accepted")
	}
}

func TestWebPUpload(t *testing.T) {
	// Generated locally from a solid color with FFmpeg; no third-party image content.
	raw, err := os.ReadFile("testdata/generated.webp")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Prepare(context.Background(), bytes.NewReader(raw), "print.webp", "image/webp", Limits{MaxBytes: 4096, MaxPixels: 100, MaxDimension: 100})
	if err != nil {
		t.Fatal(err)
	}
	if result.Width != 4 || result.Height != 4 || result.MediaType != "image/webp" || !bytes.Equal(result.Original, raw) {
		t.Fatal("WebP original changed")
	}
}
