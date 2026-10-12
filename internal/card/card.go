// Package card renders a shareable 1200×675 PNG of a trader's stats in the
// FomoCLI look (ice white on near-black), sized for X / Telegram previews.
package card

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed logo.png
var logoPNG []byte

// Window is the PnL for one time window.
type Window struct {
	Label string
	PnL   float64
	OK    bool
}

// Stat is a labeled value in the bottom row.
type Stat struct {
	Label, Value string
	Tone         int // -1 red, 0 neutral, 1 green
}

// Data is everything drawn on the card.
type Data struct {
	Handle      string
	DisplayName string
	Verified    bool
	Subtitle    string   // e.g. "Ender clan · 360 days on FOMO"
	Headline    Window   // the big number
	Windows     []Window // bar chart
	Stats       []Stat   // bottom tiles (up to 5)
	Footer      string
	Demo        bool
}

const W, H = 1200, 675

var (
	ink    = color.RGBA{5, 6, 15, 255}
	panel  = color.RGBA{14, 17, 32, 255}
	edge   = color.RGBA{36, 41, 66, 255}
	ice    = color.RGBA{238, 241, 252, 255}
	steel  = color.RGBA{170, 178, 214, 255}
	muted  = color.RGBA{110, 117, 150, 255}
	green  = color.RGBA{94, 230, 160, 255}
	red    = color.RGBA{255, 107, 129, 255}
	gold   = color.RGBA{242, 210, 139, 255}
	faceMu sync.Mutex
	faces  = map[string]font.Face{}
)

func face(ttf []byte, name string, size float64) font.Face {
	faceMu.Lock()
	defer faceMu.Unlock()
	key := fmt.Sprintf("%s/%.0f", name, size)
	if f, ok := faces[key]; ok {
		return f
	}
	ft, err := opentype.Parse(ttf)
	if err != nil {
		panic(err)
	}
	f, err := opentype.NewFace(ft, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	faces[key] = f
	return f
}

func bold(size float64) font.Face    { return face(gobold.TTF, "bold", size) }
func medium(size float64) font.Face  { return face(gomedium.TTF, "medium", size) }
func regular(size float64) font.Face { return face(goregular.TTF, "regular", size) }

// Render draws the card.
func Render(d Data) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	fill(img, img.Bounds(), ink)

	// subtle dot grid
	for y := 20; y < H; y += 28 {
		for x := 20; x < W; x += 28 {
			blend(img, x, y, color.RGBA{238, 241, 252, 255}, 0.06)
		}
	}

	// logo + tag
	if logo, err := png.Decode(bytes.NewReader(logoPNG)); err == nil {
		lw := 190
		lh := lw * logo.Bounds().Dy() / logo.Bounds().Dx()
		dst := image.Rect(64, 52, 64+lw, 52+lh)
		xdraw.CatmullRom.Scale(img, dst, logo, logo.Bounds(), xdraw.Over, nil)
	}
	tag := "TRADER CARD"
	if d.Demo {
		tag = "DEMO DATA · TRADER CARD"
	}
	tagC := muted
	if d.Demo {
		tagC = gold
	}
	text(img, regular(18), tagC, W-64-measure(regular(18), tag), 92, tag)

	// identity
	handle := "@" + d.Handle
	hf := fitFace(handle, 620, 64, 36)
	text(img, hf, ice, 64, 220, handle)
	if d.Verified {
		cx := 64 + measure(hf, handle) + 30
		check(img, cx, 220-int(hf.Metrics().Ascent.Round())/2+2, 15)
	}
	sub := d.DisplayName
	if d.Subtitle != "" {
		if sub != "" {
			sub += "  ·  "
		}
		sub += d.Subtitle
	}
	text(img, medium(24), steel, 66, 262, truncate(medium(24), sub, 600))

	// headline pnl
	text(img, medium(20), muted, 66, 336, strings.ToUpper("PnL · "+d.Headline.Label))
	big := signedUSD(d.Headline.PnL)
	col := tone(d.Headline.PnL)
	if !d.Headline.OK {
		big, col = "—", muted
	}
	text(img, bold(104), col, 60, 446, big)

	// right panel: pnl by window
	px, py, pw, ph := 700, 150, 436, 312
	roundRect(img, image.Rect(px, py, px+pw, py+ph), 18, panel, edge)
	text(img, medium(18), muted, px+28, py+44, "PNL BY WINDOW")
	var maxAbs float64
	for _, w := range d.Windows {
		if w.OK {
			maxAbs = math.Max(maxAbs, math.Abs(w.PnL))
		}
	}
	for i, w := range d.Windows {
		y := py + 84 + i*56
		text(img, medium(20), steel, px+28, y+18, w.Label)
		val := "—"
		c := muted
		if w.OK {
			val, c = signedUSD(w.PnL), tone(w.PnL)
		}
		vf := medium(20)
		text(img, vf, c, px+pw-28-measure(vf, val), y+18, val)
		barX, barW := px+92, pw-92-28-110
		roundRect(img, image.Rect(barX, y+4, barX+barW, y+20), 8, color.RGBA{24, 28, 48, 255}, color.RGBA{})
		if w.OK && maxAbs > 0 {
			bw := int(float64(barW) * math.Abs(w.PnL) / maxAbs)
			if bw < 10 {
				bw = 10
			}
			roundRect(img, image.Rect(barX, y+4, barX+bw, y+20), 8, c, color.RGBA{})
		}
	}

	// bottom stat tiles
	n := len(d.Stats)
	if n > 5 {
		n = 5
	}
	if n > 0 {
		gap := 16
		tw := (W - 128 - gap*(n-1)) / n
		for i := 0; i < n; i++ {
			s := d.Stats[i]
			x := 64 + i*(tw+gap)
			r := image.Rect(x, 500, x+tw, 600)
			roundRect(img, r, 16, panel, edge)
			text(img, medium(16), muted, x+22, 534, strings.ToUpper(s.Label))
			c := ice
			switch s.Tone {
			case 1:
				c = green
			case -1:
				c = red
			}
			text(img, fitFace(s.Value, tw-44, 34, 20), c, x+22, 580, s.Value)
		}
	}

	// footer
	text(img, regular(17), muted, 64, 640, d.Footer)
	brand := "github.com/FomoCLI/FomoCLI"
	text(img, medium(17), steel, W-64-measure(medium(17), brand), 640, brand)
	return img
}

// WritePNG encodes the card.
func WritePNG(w io.Writer, img image.Image) error { return png.Encode(w, img) }

// ---------------------------------------------------------------- helpers

func tone(f float64) color.RGBA {
	switch {
	case f > 0:
		return green
	case f < 0:
		return red
	}
	return ice
}

// SignedUSD formats +$151.4K / -$3.1K.
func SignedUSD(f float64) string { return signedUSD(f) }

func signedUSD(f float64) string {
	sign := "+"
	if f < 0 {
		sign, f = "-", -f
	}
	if f == 0 {
		return "$0"
	}
	return sign + compactUSD(f)
}

// CompactUSD formats $1.2M.
func CompactUSD(f float64) string { return compactUSD(f) }

func compactUSD(f float64) string {
	a := math.Abs(f)
	s := ""
	switch {
	case a >= 1e9:
		s = trimf(a/1e9) + "B"
	case a >= 1e6:
		s = trimf(a/1e6) + "M"
	case a >= 1e3:
		s = trimf(a/1e3) + "K"
	default:
		s = fmt.Sprintf("%.0f", a)
	}
	if f < 0 {
		return "-$" + s
	}
	return "$" + s
}

func trimf(f float64) string { return strings.TrimSuffix(fmt.Sprintf("%.1f", f), ".0") }

func text(img *image.RGBA, f font.Face, c color.Color, x, y int, s string) {
	d := &font.Drawer{Dst: img, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

func measure(f font.Face, s string) int { return font.MeasureString(f, s).Round() }

// fitFace picks the largest bold size (max→min) that fits s in width.
func fitFace(s string, width int, max, min float64) font.Face {
	for size := max; size > min; size -= 2 {
		if measure(bold(size), s) <= width {
			return bold(size)
		}
	}
	return bold(min)
}

func truncate(f font.Face, s string, width int) string {
	if measure(f, s) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && measure(f, string(r)+"…") > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func fill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func blend(img *image.RGBA, x, y int, c color.RGBA, a float64) {
	if !(image.Point{x, y}.In(img.Bounds())) {
		return
	}
	o := img.RGBAAt(x, y)
	mix := func(b, f uint8) uint8 { return uint8(float64(b)*(1-a) + float64(f)*a) }
	img.SetRGBA(x, y, color.RGBA{mix(o.R, c.R), mix(o.G, c.G), mix(o.B, c.B), 255})
}

// roundRect fills a rounded rectangle with an optional 1px border (zero
// border color = no border), antialiasing the corners.
func roundRect(img *image.RGBA, r image.Rectangle, rad int, fillC, border color.RGBA) {
	rf := float64(rad)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			// distance outside the rounded shape (negative = inside)
			cx := math.Max(float64(r.Min.X)+rf, math.Min(float64(x)+0.5, float64(r.Max.X)-rf))
			cy := math.Max(float64(r.Min.Y)+rf, math.Min(float64(y)+0.5, float64(r.Max.Y)-rf))
			dist := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) - rf
			if dist > 0.5 {
				continue
			}
			cov := math.Min(1, 0.5-dist)
			c := fillC
			if border.A > 0 && dist > -1.5 {
				c = border
			}
			blend(img, x, y, c, cov)
		}
	}
}

func check(img *image.RGBA, cx, cy, r int) {
	roundRect(img, image.Rect(cx-r, cy-r, cx+r, cy+r), r, ice, color.RGBA{})
	// draw a thick check mark
	pts := [][2]float64{{-0.45, 0.02}, {-0.12, 0.35}, {0.48, -0.32}}
	for i := 0; i < len(pts)-1; i++ {
		x0, y0 := float64(cx)+pts[i][0]*float64(r), float64(cy)+pts[i][1]*float64(r)
		x1, y1 := float64(cx)+pts[i+1][0]*float64(r), float64(cy)+pts[i+1][1]*float64(r)
		steps := 60
		for s := 0; s <= steps; s++ {
			t := float64(s) / float64(steps)
			x, y := x0+(x1-x0)*t, y0+(y1-y0)*t
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					if dx*dx+dy*dy <= 4 {
						img.SetRGBA(int(x)+dx, int(y)+dy, ink)
					}
				}
			}
		}
	}
}
