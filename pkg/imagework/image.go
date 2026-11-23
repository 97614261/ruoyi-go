// Package imagework implements bounded raster operations without storage or network access.
package imagework

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"strconv"

	xdraw "golang.org/x/image/draw"
)

const MaxPixels = 24_000_000

type Config struct {
	Operation     string  `json:"operation"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	Scale         int     `json:"scale"`
	Rotate        int     `json:"rotate"`
	CropX         float64 `json:"cropX"`
	CropY         float64 `json:"cropY"`
	CropWidth     float64 `json:"cropWidth"`
	CropHeight    float64 `json:"cropHeight"`
	Background    string  `json:"background"`
	Tolerance     int     `json:"tolerance"`
	Method        string  `json:"method"`
	Product       string  `json:"product"`
	Side          string  `json:"side"`
	Color         string  `json:"color"`
	PrintX        float64 `json:"printX"`
	PrintY        float64 `json:"printY"`
	PrintScale    float64 `json:"printScale"`
	Format        string  `json:"format"`
	FileName      string  `json:"fileName"`
	Mode          string  `json:"mode"`
	PrintWidthMM  float64 `json:"printWidthMM"`
	PrintHeightMM float64 `json:"printHeightMM"`
}

func Parse(raw json.RawMessage) (Config, error) {
	c := Config{Operation: "resize", Width: 2048, Height: 2048, Scale: 2, CropWidth: 1, CropHeight: 1, Background: "#ffffff", Tolerance: 30, Method: "basic", Product: "tshirt", Side: "front", Color: "#ffffff", PrintX: .5, PrintY: .4, PrintScale: .6, Format: "png", FileName: "POD", Mode: "print", PrintWidthMM: 300, PrintHeightMM: 400}
	err := json.Unmarshal(raw, &c)
	return c, err
}

func Dimensions(w, h int) error {
	if w < 1 || h < 1 || w > 8192 || h > 8192 || int64(w)*int64(h) > MaxPixels {
		return errors.New("输出最多 8192 像素边长、2400 万像素，请减小尺寸或放大倍数")
	}
	return nil
}

func Decode(raw []byte) (image.Image, error) {
	c, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("无法读取输入图片")
	}
	if err = Dimensions(c.Width, c.Height); err != nil {
		return nil, err
	}
	im, _, err := image.Decode(bytes.NewReader(raw))
	return im, err
}

func Process(ctx context.Context, src image.Image, c Config) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Operation == "crop" || c.Operation == "resize" {
		src = Rotate(src, c.Rotate)
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	switch c.Operation {
	case "crop":
		if c.CropX < 0 || c.CropY < 0 || c.CropWidth <= 0 || c.CropHeight <= 0 || c.CropX+c.CropWidth > 1.000001 || c.CropY+c.CropHeight > 1.000001 {
			return nil, errors.New("裁剪区域超出图片")
		}
		r := image.Rect(int(c.CropX*float64(w)), int(c.CropY*float64(h)), int((c.CropX+c.CropWidth)*float64(w)), int((c.CropY+c.CropHeight)*float64(h)))
		if r.Empty() {
			return nil, errors.New("裁剪区域太小")
		}
		out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
		draw.Draw(out, out.Bounds(), src, src.Bounds().Min.Add(r.Min), draw.Src)
		return out, ctx.Err()
	case "resize", "upscale":
		if c.Operation == "upscale" {
			c.Width = w * c.Scale
			c.Height = h * c.Scale
		}
		if err := Dimensions(c.Width, c.Height); err != nil {
			return nil, err
		}
		// Resize fits inside the requested box; it never silently stretches the artwork.
		ratio := math.Min(float64(c.Width)/float64(w), float64(c.Height)/float64(h))
		out := image.NewNRGBA(image.Rect(0, 0, max(1, int(math.Round(float64(w)*ratio))), max(1, int(math.Round(float64(h)*ratio)))))
		xdraw.CatmullRom.Scale(out, out.Bounds(), src, src.Bounds(), draw.Src, nil)
		return out, ctx.Err()
	case "removeBackground":
		return RemoveBackground(ctx, src, Hex(c.Background), c.Tolerance)
	default:
		return nil, errors.New("不支持的图片处理方式")
	}
}

func Rotate(src image.Image, degrees int) image.Image {
	if degrees == 0 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	ow, oh := w, h
	if degrees == 90 || degrees == 270 {
		ow, oh = h, w
	}
	out := image.NewNRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch degrees {
			case 90:
				dx, dy = h-1-y, x
			case 180:
				dx, dy = w-1-x, h-1-y
			case 270:
				dx, dy = y, w-1-x
			}
			out.Set(dx, dy, src.At(x+src.Bounds().Min.X, y+src.Bounds().Min.Y))
		}
	}
	return out
}

// Only matching background connected to an image border is removed, preserving enclosed details.
func RemoveBackground(ctx context.Context, src image.Image, bg color.NRGBA, tolerance int) (image.Image, error) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Bounds(), src, src.Bounds().Min, draw.Src)
	seen := make([]bool, w*h)
	queue := make([]int32, 0, w*h/4)
	visit := func(x, y int) {
		i := y*w + x
		if seen[i] {
			return
		}
		seen[i] = true
		p := out.NRGBAAt(x, y)
		d := max(abs(int(p.R)-int(bg.R)), abs(int(p.G)-int(bg.G)), abs(int(p.B)-int(bg.B)))
		if int(p.A) == 0 || d <= tolerance {
			queue = append(queue, int32(i))
		}
	}
	for x := 0; x < w; x++ {
		visit(x, 0)
		visit(x, h-1)
	}
	for y := 0; y < h; y++ {
		visit(0, y)
		visit(w-1, y)
	}
	for head := 0; head < len(queue); head++ {
		if head%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		i := int(queue[head])
		x, y := i%w, i/w
		out.Pix[y*out.Stride+x*4+3] = 0
		if x > 0 {
			visit(x-1, y)
		}
		if x+1 < w {
			visit(x+1, y)
		}
		if y > 0 {
			visit(x, y-1)
		}
		if y+1 < h {
			visit(x, y+1)
		}
	}
	return out, nil
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func Hex(s string) color.NRGBA {
	n, _ := strconv.ParseUint(trimHash(s), 16, 32)
	return color.NRGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}
}
func trimHash(s string) string {
	if len(s) == 7 && s[0] == '#' {
		return s[1:]
	}
	return "ffffff"
}

func Encode(ctx context.Context, src image.Image, format string) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	var err error
	media := "image/png"
	switch format {
	case "png":
		err = png.Encode(&buf, src)
	case "jpeg":
		flat := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
		draw.Draw(flat, flat.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		draw.Draw(flat, flat.Bounds(), src, src.Bounds().Min, draw.Over)
		err = jpeg.Encode(&buf, flat, &jpeg.Options{Quality: 95})
		media = "image/jpeg"
	default:
		return nil, "", errors.New("当前导出支持 PNG 和 JPEG，请将旧 WebP 配置改为 PNG")
	}
	if err != nil {
		return nil, "", err
	}
	return buf.Bytes(), media, ctx.Err()
}

func EffectiveDPI(w, h int, mmW, mmH float64) float64 {
	return math.Floor(math.Min(float64(w)*25.4/mmW, float64(h)*25.4/mmH))
}
