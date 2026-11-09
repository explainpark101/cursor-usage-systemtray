package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"math"
)

// 3x5 pixel font; drawn with glyphScale so digits fill a 32x32 tray icon.
var glyphs = map[rune][5]string{
	'0': {"###", "#.#", "#.#", "#.#", "###"},
	'1': {".#.", "##.", ".#.", ".#.", "###"},
	'2': {"###", "..#", "###", "#..", "###"},
	'3': {"###", "..#", ".##", "..#", "###"},
	'4': {"#.#", "#.#", "###", "..#", "..#"},
	'5': {"###", "#..", "###", "..#", "###"},
	'6': {"###", "#..", "###", "#.#", "###"},
	'7': {"###", "..#", ".#.", ".#.", ".#."},
	'8': {"###", "#.#", "###", "#.#", "###"},
	'9': {"###", "#.#", "###", "..#", "###"},
	'%': {"#.#", "..#", ".#.", "#..", "#.#"},
	'-': {"...", "...", "###", "...", "..."},
	'!': {".#.", ".#.", ".#.", "...", ".#."},
	' ': {"...", "...", "...", "...", "..."},
}

var (
	bgColor     = color.RGBA{0x1c, 0x1c, 0x20, 0xff}
	fgColor     = color.RGBA{0xff, 0xff, 0xff, 0xff}
	normalColor = color.RGBA{0x4a, 0xde, 0x80, 0xff}
	warnColor   = color.RGBA{0xfa, 0xcc, 0x15, 0xff}
	dangerColor = color.RGBA{0xf8, 0x71, 0x71, 0xff}
	errorColor  = color.RGBA{0xf8, 0x71, 0x71, 0xff}
	idleColor   = color.RGBA{0x88, 0x88, 0x90, 0xff}
	spinColor   = color.RGBA{0x93, 0xc5, 0xfd, 0xff}
)

// 32x32 matches Windows SM_CXICON used by systray LoadImage(LR_DEFAULTSIZE).
const iconSize = 32
const glyphScale = 3 // 3x5 → 9x15 px digits
const glyphW = 3
const glyphH = 5
const spinnerFrames = 8
const barH = 4

func barColor(percent int) color.RGBA {
	switch {
	case percent >= 90:
		return dangerColor
	case percent >= 70:
		return warnColor
	default:
		return normalColor
	}
}

func scaledGlyphSize(n int) (w, h, gap int) {
	h = glyphH * glyphScale
	gap = 1
	if n <= 0 {
		return 0, h, gap
	}
	w = n*(glyphW*glyphScale) + (n-1)*gap
	return w, h, gap
}

func drawGlyph(img *image.RGBA, x0, y0 int, r rune, fg color.RGBA) {
	g, ok := glyphs[r]
	if !ok {
		g = glyphs[' ']
	}
	for y, row := range g {
		for x := 0; x < glyphW; x++ {
			if row[x] != '#' {
				continue
			}
			for dy := 0; dy < glyphScale; dy++ {
				for dx := 0; dx < glyphScale; dx++ {
					img.Set(x0+x*glyphScale+dx, y0+y*glyphScale+dy, fg)
				}
			}
		}
	}
}

// renderIcon draws a 32x32 ICO: large percent digits plus a color bar.
func renderIcon(text string, percent int, fg color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	runes := []rune(text)
	tw, th, gap := scaledGlyphSize(len(runes))
	x0 := (iconSize - tw) / 2
	y0 := (iconSize - barH - th) / 2
	if y0 < 1 {
		y0 = 1
	}
	step := glyphW*glyphScale + gap
	for i, r := range runes {
		drawGlyph(img, x0+i*step, y0, r, fg)
	}

	if percent >= 0 {
		c := barColor(percent)
		w := percent * iconSize / 100
		if w < 1 && percent > 0 {
			w = 1
		}
		for y := iconSize - barH; y < iconSize; y++ {
			for x := 0; x < w; x++ {
				img.Set(x, y, c)
			}
		}
	}

	return encodeICO(img)
}

// renderSpinnerFrame draws one frame of a rotating arc spinner as a 32x32 ICO.
func renderSpinnerFrame(frame int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	cx, cy := float64(iconSize-1)/2, float64(iconSize-1)/2
	radius := 11.0
	start := float64(frame) / float64(spinnerFrames) * 2 * math.Pi
	const steps = 32
	const sweep = steps * 3 / 4

	for i := 0; i < sweep; i++ {
		ang := start + float64(i)/float64(steps)*2*math.Pi
		f := float64(i) / float64(sweep-1)
		c := color.RGBA{
			uint8(float64(spinColor.R) * (0.35 + 0.65*f)),
			uint8(float64(spinColor.G) * (0.35 + 0.65*f)),
			uint8(float64(spinColor.B) * (0.35 + 0.65*f)),
			0xff,
		}
		x := int(math.Round(cx + radius*math.Cos(ang)))
		y := int(math.Round(cy + radius*math.Sin(ang)))
		// 2x2 blob so the arc reads clearly when downscaled by the shell.
		for dy := 0; dy < 2; dy++ {
			for dx := 0; dx < 2; dx++ {
				px, py := x+dx, y+dy
				if px >= 0 && px < iconSize && py >= 0 && py < iconSize {
					img.Set(px, py, c)
				}
			}
		}
	}

	return encodeICO(img)
}

func encodeICO(img *image.RGBA) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	xorSize := w * h * 4
	andRow := ((w + 31) / 32) * 4
	andSize := andRow * h
	dibSize := 40 + xorSize + andSize

	buf := bytes.NewBuffer(make([]byte, 0, 6+16+dibSize))
	_ = binary.Write(buf, binary.LittleEndian, uint16(0))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))

	widthByte := byte(w)
	heightByte := byte(h)
	if w >= 256 {
		widthByte = 0
	}
	if h >= 256 {
		heightByte = 0
	}
	buf.WriteByte(widthByte)
	buf.WriteByte(heightByte)
	buf.WriteByte(0)
	buf.WriteByte(0)
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(32))
	_ = binary.Write(buf, binary.LittleEndian, uint32(dibSize))
	_ = binary.Write(buf, binary.LittleEndian, uint32(22))

	_ = binary.Write(buf, binary.LittleEndian, uint32(40))
	_ = binary.Write(buf, binary.LittleEndian, int32(w))
	_ = binary.Write(buf, binary.LittleEndian, int32(h*2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(32))
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(buf, binary.LittleEndian, uint32(xorSize+andSize))
	_ = binary.Write(buf, binary.LittleEndian, int32(0))
	_ = binary.Write(buf, binary.LittleEndian, int32(0))
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))

	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			off := img.PixOffset(x, y)
			buf.WriteByte(img.Pix[off+2])
			buf.WriteByte(img.Pix[off+1])
			buf.WriteByte(img.Pix[off+0])
			buf.WriteByte(img.Pix[off+3])
		}
	}
	buf.Write(make([]byte, andSize))
	return buf.Bytes()
}
