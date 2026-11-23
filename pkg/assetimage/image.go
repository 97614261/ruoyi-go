// Package assetimage validates uploaded raster images before full decoding.
package assetimage

import (
	"bytes"
	"context"
	"errors"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Limits struct {
	MaxBytes, MaxPixels int64
	MaxDimension        int
}
type Prepared struct {
	Original, Thumbnail                    []byte
	Name, MediaType, Extension             string
	Width, Height, ThumbWidth, ThumbHeight int
}

var ErrImage = errors.New("图片格式不合法，请上传完整的 PNG、JPEG 或 WebP 图片")
var ErrSize = errors.New("图片字节数、像素数或边长超过上限")
var ErrName = errors.New("文件名须为 1 到 180 个字符，不能包含路径或控制字符")

func Prepare(ctx context.Context, r io.Reader, name, declared string, limits Limits) (*Prepared, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 180 || strings.ContainsAny(name, `/\:`) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return nil, ErrName
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(r, limits.MaxBytes+1))
	if err != nil {
		return nil, ErrImage
	}
	if int64(len(raw)) > limits.MaxBytes {
		return nil, ErrSize
	}
	header, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrImage
	}
	media, ext := "", ""
	switch format {
	case "png":
		media, ext = "image/png", "png"
	case "jpeg":
		media, ext = "image/jpeg", "jpg"
	case "webp":
		media, ext = "image/webp", "webp"
	default:
		return nil, ErrImage
	}
	extension := strings.ToLower(filepath.Ext(name))
	if extension != "."+ext && !(ext == "jpg" && extension == ".jpeg") {
		return nil, ErrImage
	}
	if declared != "" {
		kind, _, err := mime.ParseMediaType(declared)
		if err != nil || (kind != media && kind != "application/octet-stream") {
			return nil, ErrImage
		}
	}
	if header.Width < 1 || header.Height < 1 || header.Width > limits.MaxDimension || header.Height > limits.MaxDimension || int64(header.Width)*int64(header.Height) > limits.MaxPixels {
		return nil, ErrSize
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	decoded, actual, err := image.Decode(bytes.NewReader(raw))
	if err != nil || actual != format {
		return nil, ErrImage
	}
	if decoded.Bounds().Dx() != header.Width || decoded.Bounds().Dy() != header.Height {
		return nil, ErrImage
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	width, height := header.Width, header.Height
	if width > 480 || height > 480 {
		if width >= height {
			height = max(1, height*480/width)
			width = 480
		} else {
			width = max(1, width*480/height)
			height = 480
		}
	}
	thumbnail := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.ApproxBiLinear.Scale(thumbnail, thumbnail.Bounds(), decoded, decoded.Bounds(), draw.Src, nil)
	var output bytes.Buffer
	if err := png.Encode(&output, thumbnail); err != nil {
		return nil, ErrImage
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Prepared{Original: raw, Thumbnail: output.Bytes(), Name: name, MediaType: media, Extension: ext, Width: header.Width, Height: header.Height, ThumbWidth: width, ThumbHeight: height}, nil
}
