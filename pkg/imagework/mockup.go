package imagework

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/vector"
)

// Geometry is shared with studio/src/ProductLayout.vue (1000 x 1200 viewBox).
var PrintArea = image.Rect(325, 300, 675, 850)
var outline = [][2]float32{{350, 120}, {430, 100}, {570, 100}, {650, 120}, {870, 300}, {755, 445}, {685, 390}, {685, 1060}, {315, 1060}, {315, 390}, {245, 445}, {130, 300}}

func Mockup(ctx context.Context, artwork image.Image, c Config) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := image.NewNRGBA(image.Rect(0, 0, 1000, 1200))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.NRGBA{R: 241, G: 244, B: 248, A: 255}), image.Point{}, draw.Src)
	r := vector.NewRasterizer(1000, 1200)
	r.MoveTo(outline[0][0], outline[0][1])
	for _, p := range outline[1:] {
		r.LineTo(p[0], p[1])
	}
	r.ClosePath()
	r.Draw(out, out.Bounds(), image.NewUniform(Hex(c.Color)), image.Point{})
	// Front/back neckline and simple seams, drawn procedurally rather than using third-party mockups.
	neck := vector.NewRasterizer(1000, 1200)
	neck.MoveTo(430, 100)
	if c.Side == "back" {
		neck.QuadTo(500, 145, 570, 100)
	} else {
		neck.CubeTo(440, 220, 560, 220, 570, 100)
	}
	neck.ClosePath()
	neck.Draw(out, out.Bounds(), image.NewUniform(color.NRGBA{R: 225, G: 230, B: 236, A: 255}), image.Point{})
	seam := color.NRGBA{R: 130, G: 140, B: 150, A: 70}
	for x := 330; x < 670; x++ {
		out.SetNRGBA(x, 1040, seam)
	}
	for y := 410; y < 1030; y++ {
		out.SetNRGBA(330, y, seam)
		out.SetNRGBA(670, y, seam)
	}
	area := PrintArea
	factor := math.Min(float64(area.Dx())/float64(artwork.Bounds().Dx()), float64(area.Dy())/float64(artwork.Bounds().Dy())) * math.Min(1, c.PrintScale)
	w, h := max(1, int(math.Round(float64(artwork.Bounds().Dx())*factor))), max(1, int(math.Round(float64(artwork.Bounds().Dy())*factor)))
	x := area.Min.X + int(math.Round(float64(area.Dx()-w)*c.PrintX))
	y := area.Min.Y + int(math.Round(float64(area.Dy()-h)*c.PrintY))
	xdraw.CatmullRom.Scale(out, image.Rect(x, y, x+w, y+h), artwork, artwork.Bounds(), draw.Over, nil)
	return out, ctx.Err()
}
