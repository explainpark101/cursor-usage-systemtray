// makeicon builds a multi-resolution Windows .ico from assets/icon.png.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

var sizes = []int{16, 24, 32, 48, 64, 128, 256}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	srcPath := filepath.Join(root, "assets", "icon.png")
	dstPath := filepath.Join(root, "assets", "icon.ico")

	f, err := os.Open(srcPath)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		fatal(err)
	}

	var pngs [][]byte
	for _, sz := range sizes {
		img := resizeRGBA(src, sz)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			fatal(err)
		}
		pngs = append(pngs, buf.Bytes())
	}

	ico, err := encodeICO(pngs)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(dstPath, ico, 0o644); err != nil {
		fatal(err)
	}
	fmt.Println("wrote", dstPath)
}

func resizeRGBA(src image.Image, size int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			// Bilinear sample.
			fx := (float64(x)+0.5)*float64(sw)/float64(size) - 0.5
			fy := (float64(y)+0.5)*float64(sh)/float64(size) - 0.5
			dst.SetRGBA(x, y, sampleBilinear(src, sb.Min.X, sb.Min.Y, sw, sh, fx, fy))
		}
	}
	return dst
}

func sampleBilinear(src image.Image, ox, oy, sw, sh int, fx, fy float64) color.RGBA {
	if fx < 0 {
		fx = 0
	}
	if fy < 0 {
		fy = 0
	}
	if fx > float64(sw-1) {
		fx = float64(sw - 1)
	}
	if fy > float64(sh-1) {
		fy = float64(sh - 1)
	}
	x0 := int(fx)
	y0 := int(fy)
	x1 := x0 + 1
	y1 := y0 + 1
	if x1 >= sw {
		x1 = sw - 1
	}
	if y1 >= sh {
		y1 = sh - 1
	}
	dx := fx - float64(x0)
	dy := fy - float64(y0)

	c00 := rgbaAt(src, ox+x0, oy+y0)
	c10 := rgbaAt(src, ox+x1, oy+y0)
	c01 := rgbaAt(src, ox+x0, oy+y1)
	c11 := rgbaAt(src, ox+x1, oy+y1)

	return color.RGBA{
		R: lerpByte(lerpByte(c00.R, c10.R, dx), lerpByte(c01.R, c11.R, dx), dy),
		G: lerpByte(lerpByte(c00.G, c10.G, dx), lerpByte(c01.G, c11.G, dx), dy),
		B: lerpByte(lerpByte(c00.B, c10.B, dx), lerpByte(c01.B, c11.B, dx), dy),
		A: lerpByte(lerpByte(c00.A, c10.A, dx), lerpByte(c01.A, c11.A, dx), dy),
	}
}

func rgbaAt(src image.Image, x, y int) color.RGBA {
	r, g, b, a := src.At(x, y).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func lerpByte(a, b uint8, t float64) uint8 {
	return uint8(float64(a)*(1-t) + float64(b)*t + 0.5)
}

func encodeICO(pngs [][]byte) ([]byte, error) {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(len(pngs)))

	offset := 6 + 16*len(pngs)
	for _, p := range pngs {
		cfg, err := png.DecodeConfig(bytes.NewReader(p))
		if err != nil {
			return nil, err
		}
		w, h := cfg.Width, cfg.Height
		if w >= 256 {
			w = 0
		}
		if h >= 256 {
			h = 0
		}
		buf.WriteByte(byte(w))
		buf.WriteByte(byte(h))
		buf.WriteByte(0)
		buf.WriteByte(0)
		_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
		_ = binary.Write(&buf, binary.LittleEndian, uint16(32))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(len(p)))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(p)
	}
	for _, p := range pngs {
		buf.Write(p)
	}
	return buf.Bytes(), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
