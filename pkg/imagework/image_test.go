package imagework

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRasterOperations(t *testing.T) {
	ctx := context.Background()
	src := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	src.SetNRGBA(10, 5, color.NRGBA{R: 255, A: 128})
	c, _ := Parse([]byte(`{"width":100,"height":100}`))
	out, err := Process(ctx, src, c)
	if err != nil || out.Bounds().Dx() != 100 || out.Bounds().Dy() != 50 {
		t.Fatal("aspect ratio not preserved", err)
	}
	c, _ = Parse([]byte(`{"operation":"crop","cropX":0.25,"cropY":0.1,"cropWidth":0.5,"cropHeight":0.5}`))
	out, err = Process(ctx, src, c)
	if err != nil || out.Bounds().Dx() != 100 || out.Bounds().Dy() != 50 {
		t.Fatal("crop", err)
	}
	c.CropX = .8
	if _, err = Process(ctx, src, c); err == nil {
		t.Fatal("out of bounds crop accepted")
	}
	rotated := Rotate(src, 90)
	if rotated.Bounds().Dx() != 100 || rotated.Bounds().Dy() != 200 || color.NRGBAModel.Convert(rotated.At(94, 10)).(color.NRGBA).A != 128 {
		t.Fatal("clockwise rotation or alpha lost")
	}
	if Dimensions(8192, 8192) == nil || Dimensions(0, 10) == nil {
		t.Fatal("pixel budget not enforced")
	}
	c, _ = Parse([]byte(`{"operation":"upscale","scale":4}`))
	if _, err = Process(ctx, image.NewNRGBA(image.Rect(0, 0, 2000, 2000)), c); err == nil {
		t.Fatal("oversized scale accepted")
	}
}

func TestBackgroundPreservesEnclosedWhiteAndAlpha(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for y := 5; y < 15; y++ {
		for x := 5; x < 15; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	src.SetNRGBA(10, 10, color.NRGBA{R: 255, G: 255, B: 255, A: 180})
	out, err := RemoveBackground(context.Background(), src, Hex("#ffffff"), 20)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := out.At(0, 0).RGBA(); a != 0 {
		t.Fatal("border background not removed")
	}
	if _, _, _, a := out.At(10, 10).RGBA(); a != 180*257 {
		t.Fatal("enclosed detail or alpha lost")
	}
	raw, _, err := Encode(context.Background(), out, "png")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := decoded.At(0, 0).RGBA(); a != 0 {
		t.Fatal("png lost transparency")
	}
	raw, _, err = Encode(context.Background(), out, "jpeg")
	if err != nil {
		t.Fatal(err)
	}
	j, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := j.At(0, 0).RGBA()
	if r < 60000 || g < 60000 || b < 60000 {
		t.Fatal("jpeg not flattened on white")
	}
}

func TestMockupAndPrintMetrics(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 200))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.NRGBA{R: 250, A: 255}), image.Point{}, draw.Src)
	c, _ := Parse([]byte(`{"printScale":1,"printX":1,"printY":1,"color":"#123456"}`))
	out, err := Mockup(context.Background(), src, c)
	if err != nil {
		t.Fatal(err)
	}
	if out.Bounds() != image.Rect(0, 0, 1000, 1200) {
		t.Fatal("mockup dimensions")
	}
	for y := 0; y < 1200; y++ {
		for x := 0; x < 1000; x++ {
			p := color.NRGBAModel.Convert(out.At(x, y)).(color.NRGBA)
			if p.R == 250 && p.G == 0 && p.B == 0 && !image.Pt(x, y).In(PrintArea) {
				t.Fatal("artwork outside print area")
			}
		}
	}
	front := out.At(500, 160)
	c.Side = "back"
	back, _ := Mockup(context.Background(), src, c)
	if front == back.At(500, 160) {
		t.Fatal("front/back neckline identical")
	}
	if dpi := EffectiveDPI(3000, 4000, 254, 338.6666667); dpi != 299 && dpi != 300 {
		t.Fatal("effective dpi", dpi)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = Mockup(ctx, src, c); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestLocalAITransportBoundaries(t *testing.T) {
	for _, raw := range []string{"https://example.com", "http://localhost:18112", "http://127.0.0.1:18112/path", "http://127.0.0.1:18112?token=x", "http://user@127.0.0.1:18112"} {
		if ValidAIURL(raw) {
			t.Fatal("unsafe url", raw)
		}
	}
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/octet-stream" || r.URL.Query().Get("scale") != "2" {
			t.Error("invalid protocol")
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("result"))
	}))
	defer server.Close()
	data, err := AI(context.Background(), server.URL, []byte("image"), "upscale", 2, 20)
	if err != nil || string(data) != "result" {
		t.Fatal("ai result", err)
	}
	if _, err = AI(context.Background(), server.URL, nil, "upscale", 2, 2); err == nil {
		t.Fatal("oversized response accepted")
	}
	if calls != 2 {
		t.Fatal("unexpected retry")
	}
}
