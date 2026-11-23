package imagework

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"testing"
)

func TestInteractiveCropRotatesBeforeFractionalEdges(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 7, 11))
	for y := range 11 {
		for x := range 7 {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 20), G: uint8(y * 20), A: 180})
		}
	}
	c, err := Parse([]byte(`{"operation":"crop","rotate":90,"cropX":0.15,"cropY":0.2,"cropWidth":0.5,"cropHeight":0.5}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Process(context.Background(), src, c)
	if err != nil || out.Bounds() != image.Rect(0, 0, 6, 3) {
		t.Fatal("rotated crop disagrees with displayed 6 x 3 pixels", err)
	}
	if out.At(0, 0) != src.At(1, 9) || out.At(5, 2) != src.At(3, 4) {
		t.Fatal("crop content does not use the rotated coordinate system")
	}
	raw, _, err := Encode(context.Background(), out, "png")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil || decoded.Bounds() != out.Bounds() {
		t.Fatal("download dimensions drifted", err)
	}
	if _, _, _, a := decoded.At(0, 0).RGBA(); a != 180*257 {
		t.Fatal("download lost source alpha")
	}
}

func TestInteractivePrintDefaultCenterAndLegacyScale(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 600, 800))
	red := color.NRGBA{R: 250, A: 255}
	draw.Draw(src, src.Bounds(), image.NewUniform(red), image.Point{}, draw.Src)
	for _, tc := range []struct {
		name, config string
		rect         image.Rectangle
	}{
		{"reset", `{}`, image.Rect(395, 408, 605, 688)},
		{"center", `{"printX":0.5,"printY":0.5}`, image.Rect(395, 435, 605, 715)},
		{"legacy capped scale", `{"printX":1,"printY":1,"printScale":1.5}`, image.Rect(325, 383, 675, 850)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Parse([]byte(tc.config))
			if err != nil {
				t.Fatal(err)
			}
			out, err := Mockup(context.Background(), src, c)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for y := range out.Bounds().Dy() {
				for x := range out.Bounds().Dx() {
					if color.NRGBAModel.Convert(out.At(x, y)).(color.NRGBA) == red {
						count++
						if !image.Pt(x, y).In(tc.rect) {
							t.Fatal("artwork exceeds displayed rectangle", x, y)
						}
					}
				}
			}
			if count != tc.rect.Dx()*tc.rect.Dy() {
				t.Fatal("artwork rectangle differs from the creator preview", count)
			}
		})
	}
}
