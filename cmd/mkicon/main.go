// Command mkicon renders the SVPC AI application mark and writes it as a
// multi-resolution Windows icon. Run it with `go run ./cmd/mkicon`.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
)

type icoHeader struct {
	Reserved uint16
	Type     uint16
	Count    uint16
}

type icoDirEntry struct {
	Width       uint8
	Height      uint8
	ColorCount  uint8
	Reserved    uint8
	Planes      uint16
	BitCount    uint16
	SizeInBytes uint32
	ImageOffset uint32
}

// Palette lifted from the design tokens so the icon and the app agree.
var (
	canvas    = color.RGBA{0x0A, 0x0C, 0x11, 0xFF}
	surface   = color.RGBA{0x10, 0x13, 0x1A, 0xFF}
	primary   = color.RGBA{0x4C, 0xC2, 0xFF, 0xFF}
	secondary = color.RGBA{0x9B, 0x8C, 0xFF, 0xFF}
	accent    = color.RGBA{0x2D, 0xD4, 0xBF, 0xFF}
)

const master = 1024

func main() {
	out := "assets/svpc.ico"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	img := render(master)

	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	icons := make([]image.Image, 0, len(sizes))
	for _, s := range sizes {
		icons = append(icons, scale(img, s))
	}

	if err := writeICO(out, icons); err != nil {
		fmt.Fprintln(os.Stderr, "mkicon:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d sizes)\n", out, len(icons))
}

// render draws the mark: a glass disc with a hairline ring, three stacked
// bars (the "S"), an accent spark, and code brackets at the corners.
func render(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cx, cy := size/2, size/2
	r := size * 38 / 100

	// Ambient glow behind the disc.
	for i := 0; i < size/12; i++ {
		disc(img, cx, cy, r+size/12-i, color.RGBA{primary.R, primary.G, primary.B, uint8(3)})
	}

	// Glass body with a vertical falloff.
	for yy := 0; yy < size; yy++ {
		t := float64(yy) / float64(size)
		shade := color.RGBA{
			uint8(float64(surface.R)*(1-t) + float64(canvas.R)*t),
			uint8(float64(surface.G)*(1-t) + float64(canvas.G)*t),
			uint8(float64(surface.B)*(1-t) + float64(canvas.B)*t),
			0xFF,
		}
		disc(img, cx, cy, r, shade)
	}

	// Hairline ring.
	ring(img, cx, cy, r, r-size/90, primary)

	// Inner accent ring.
	ring(img, cx, cy, r-size/12, r-size/12-size/110, accent)

	// The "S": three bars, the middle one violet with an accent centre.
	barW := r * 82 / 100
	barH := r * 15 / 100
	gap := r * 22 / 100
	left := cx - barW/2

	bar(img, left, cy-barH/2-gap-barH, barW, barH, barH/2, primary)
	bar(img, left, cy-barH/2, barW, barH, barH/2, secondary)
	bar(img, left, cy+barH/2+gap, barW, barH, barH/2, primary)

	// Accent spark on the middle bar.
	spark := r * 13 / 100
	disc(img, cx, cy-barH/2+barH/2, spark, accent)

	// Code brackets at the four corners.
	corner := r * 128 / 100
	arm := r * 20 / 100
	thick := maxInt(size/110, 1)
	bracket(img, cx-corner, cy-corner, arm, thick, 1, 1, secondary)
	bracket(img, cx+corner, cy-corner, arm, thick, -1, 1, secondary)
	bracket(img, cx-corner, cy+corner, arm, thick, 1, -1, secondary)
	bracket(img, cx+corner, cy+corner, arm, thick, -1, -1, secondary)

	return img
}

func disc(img *image.RGBA, cx, cy, r int, c color.Color) {
	if r <= 0 {
		return
	}
	for y := -r; y <= r; y++ {
		w := int(math.Sqrt(float64(r*r - y*y)))
		for x := -w; x <= w; x++ {
			px, py := cx+x, cy+y
			if px >= 0 && py >= 0 && px < img.Bounds().Dx() && py < img.Bounds().Dy() {
				img.Set(px, py, c)
			}
		}
	}
}

// ring draws an annulus between the outer and inner radius.
func ring(img *image.RGBA, cx, cy, outer, inner int, c color.Color) {
	for y := -outer; y <= outer; y++ {
		for x := -outer; x <= outer; x++ {
			d2 := x*x + y*y
			if d2 <= outer*outer && d2 >= inner*inner {
				px, py := cx+x, cy+y
				if px >= 0 && py >= 0 && px < img.Bounds().Dx() && py < img.Bounds().Dy() {
					img.Set(px, py, c)
				}
			}
		}
	}
}

// bar draws a rounded horizontal bar.
func bar(img *image.RGBA, x, y, w, h, radius int, c color.Color) {
	if radius > h/2 {
		radius = h / 2
	}
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			// Corner test: outside the quarter circle means we skip the pixel.
			out := false
			switch {
			case px < radius && py < radius:
				dx, dy := px-radius, py-radius
				out = dx*dx+dy*dy > radius*radius
			case px >= w-radius && py < radius:
				dx, dy := px-(w-radius), py-radius
				out = dx*dx+dy*dy > radius*radius
			case px < radius && py >= h-radius:
				dx, dy := px-radius, py-(h-radius)
				out = dx*dx+dy*dy > radius*radius
			case px >= w-radius && py >= h-radius:
				dx, dy := px-(w-radius), py-(h-radius)
				out = dx*dx+dy*dy > radius*radius
			}
			if !out {
				img.Set(x+px, y+py, c)
			}
		}
	}
}

// bracket draws an L-shaped code bracket. sx/sy give the corner orientation.
func bracket(img *image.RGBA, x, y, arm, thick, sx, sy int, c color.Color) {
	b := img.Bounds()
	put := func(px, py int) {
		if px >= 0 && py >= 0 && px < b.Dx() && py < b.Dy() {
			img.Set(px, py, c)
		}
	}
	for i := 0; i < arm; i++ {
		for t := 0; t < thick; t++ {
			put(x+sx*i, y+sy*t)         // vertical leg
			put(x+sx*t, y+sy*i)         // horizontal leg
			put(x+sx*(arm-1-i), y+sy*t) // other side of the vertical
		}
	}
}

func scale(src image.Image, size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	sb, db := src.Bounds(), dst.Bounds()
	for y := 0; y < db.Dy(); y++ {
		sy := sb.Min.Y + y*sb.Dy()/db.Dy()
		for x := 0; x < db.Dx(); x++ {
			sx := sb.Min.X + x*sb.Dx()/db.Dx()
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// writeICO emits a Windows icon file. Each entry is a 32-bit DIB with the
// AND mask that the format requires — omitting it is what makes GDI+ report a
// corrupt file.
func writeICO(path string, icons []image.Image) error {
	var buf bytes.Buffer

	binary.Write(&buf, binary.LittleEndian, icoHeader{
		Reserved: 0, Type: 1, Count: uint16(len(icons)),
	})

	offset := 6 + 16*len(icons)
	type entry struct {
		icoDirEntry
		data []byte
	}
	entries := make([]entry, 0, len(icons))

	for _, icon := range icons {
		b := icon.Bounds()
		w, h := b.Dx(), b.Dy()

		// Colour data: BGRA, bottom-up, 4 bytes per pixel.
		var xor bytes.Buffer
		for y := h - 1; y >= 0; y-- {
			for x := 0; x < w; x++ {
				r, g, bl, a := icon.At(x, y).RGBA()
				xor.WriteByte(byte(bl >> 8))
				xor.WriteByte(byte(g >> 8))
				xor.WriteByte(byte(r >> 8))
				xor.WriteByte(byte(a >> 8))
			}
		}

		// AND mask: 1 bit per pixel, each row padded to a 4-byte boundary.
		// Everything is 0 because the alpha channel already carries the shape.
		rowBytes := ((w + 31) / 32) * 4
		and := make([]byte, rowBytes*h)

		var img bytes.Buffer
		binary.Write(&img, binary.LittleEndian, struct {
			Size          uint32
			Width         int32
			Height        int32
			Planes        uint16
			BitCount      uint16
			Compression   uint32
			SizeImage     uint32
			XPelsPerMeter int32
			YPelsPerMeter int32
			ClrUsed       uint32
			ClrImportant  uint32
		}{
			Size:         40,
			Width:        int32(w),
			Height:       int32(h), // positive: bottom-up DIB
			Planes:       1,
			BitCount:     32,
			SizeImage:    uint32(xor.Len() + len(and)),
			ClrUsed:      0,
			ClrImportant: 0,
		})
		img.Write(xor.Bytes())
		img.Write(and)

		ew, eh := uint8(w), uint8(h)
		if w >= 256 {
			ew = 0
		}
		if h >= 256 {
			eh = 0
		}

		entries = append(entries, entry{
			icoDirEntry: icoDirEntry{
				Width: ew, Height: eh,
				Planes: 1, BitCount: 32,
				SizeInBytes: uint32(img.Len()),
				ImageOffset: uint32(offset),
			},
			data: img.Bytes(),
		})
		offset += img.Len()
	}

	for _, e := range entries {
		binary.Write(&buf, binary.LittleEndian, e.icoDirEntry)
	}
	for _, e := range entries {
		buf.Write(e.data)
	}

	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
