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
	trackColor  = color.RGBA{0x3a, 0x3a, 0x42, 0xff}
	batOutline  = color.RGBA{0xc8, 0xc8, 0xd0, 0xff}
)

// Icon style identifiers (match storage.TrayStyle*).
const (
	StyleBar     = "bar"
	StyleCircle  = "circle"
	StyleBattery = "battery"
	StyleNumber  = "number"
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

func scaledGlyphSizeAt(n, scale int) (w, h, gap int) {
	if scale < 1 {
		scale = 1
	}
	h = glyphH * scale
	gap = 1
	if n <= 0 {
		return 0, h, gap
	}
	w = n*(glyphW*scale) + (n-1)*gap
	return w, h, gap
}

func scaledGlyphSize(n int) (w, h, gap int) {
	return scaledGlyphSizeAt(n, glyphScale)
}

func drawGlyphAt(img *image.RGBA, x0, y0 int, r rune, fg color.RGBA, scale int) {
	if scale < 1 {
		scale = 1
	}
	g, ok := glyphs[r]
	if !ok {
		g = glyphs[' ']
	}
	for y, row := range g {
		for x := 0; x < glyphW; x++ {
			if row[x] != '#' {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.Set(x0+x*scale+dx, y0+y*scale+dy, fg)
				}
			}
		}
	}
}

func drawGlyph(img *image.RGBA, x0, y0 int, r rune, fg color.RGBA) {
	drawGlyphAt(img, x0, y0, r, fg, glyphScale)
}

func drawCenteredText(img *image.RGBA, text string, fg color.RGBA, scale int, yShift int) {
	runes := []rune(text)
	tw, th, gap := scaledGlyphSizeAt(len(runes), scale)
	x0 := (iconSize - tw) / 2
	y0 := (iconSize-th)/2 + yShift
	if y0 < 1 {
		y0 = 1
	}
	step := glyphW*scale + gap
	for i, r := range runes {
		drawGlyphAt(img, x0+i*step, y0, r, fg, scale)
	}
}

func newIconCanvas() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)
	return img
}

// renderIcon draws a 32x32 ICO for the given display style.
// percent < 0 skips progress chrome (idle / error text only).
func renderIcon(text string, percent int, fg color.RGBA, style string) []byte {
	if percent < 0 {
		img := newIconCanvas()
		drawCenteredText(img, text, fg, glyphScale, 0)
		return encodeICO(img)
	}

	switch style {
	case StyleCircle:
		return renderCircleIcon(text, percent, fg)
	case StyleBattery:
		return renderBatteryIcon(text, percent, fg)
	case StyleNumber:
		img := newIconCanvas()
		drawCenteredText(img, text, fg, glyphScale, 0)
		return encodeICO(img)
	default: // StyleBar
		return renderBarIcon(text, percent, fg)
	}
}

func renderBarIcon(text string, percent int, fg color.RGBA) []byte {
	img := newIconCanvas()
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

	c := barColor(percent)
	w := percent * iconSize / 100
	if w < 1 && percent > 0 {
		w = 1
	}
	if w > iconSize {
		w = iconSize
	}
	for y := iconSize - barH; y < iconSize; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return encodeICO(img)
}

func renderCircleIcon(text string, percent int, fg color.RGBA) []byte {
	img := newIconCanvas()
	cx, cy := float64(iconSize-1)/2, float64(iconSize-1)/2
	outerR := 14.0
	innerR := 10.0

	// Full track ring.
	drawRing(img, cx, cy, outerR, innerR, trackColor)

	// Progress arc starting at top (-90deg), clockwise.
	if percent > 0 {
		frac := float64(percent) / 100
		if frac > 1 {
			frac = 1
		}
		drawArcRing(img, cx, cy, outerR, innerR, -math.Pi/2, frac*2*math.Pi, barColor(percent))
	}

	scale := 2
	if len([]rune(text)) <= 2 {
		scale = 2
	}
	drawCenteredText(img, text, fg, scale, 0)
	return encodeICO(img)
}

func drawRing(img *image.RGBA, cx, cy, outerR, innerR float64, c color.RGBA) {
	minX := int(math.Floor(cx - outerR - 1))
	maxX := int(math.Ceil(cx + outerR + 1))
	minY := int(math.Floor(cy - outerR - 1))
	maxY := int(math.Ceil(cy + outerR + 1))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if x < 0 || x >= iconSize || y < 0 || y >= iconSize {
				continue
			}
			dx := float64(x) - cx
			dy := float64(y) - cy
			d := math.Hypot(dx, dy)
			if d <= outerR && d >= innerR {
				img.Set(x, y, c)
			}
		}
	}
}

func drawArcRing(img *image.RGBA, cx, cy, outerR, innerR, start, sweep float64, c color.RGBA) {
	if sweep <= 0 {
		return
	}
	minX := int(math.Floor(cx - outerR - 1))
	maxX := int(math.Ceil(cx + outerR + 1))
	minY := int(math.Floor(cy - outerR - 1))
	maxY := int(math.Ceil(cy + outerR + 1))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if x < 0 || x >= iconSize || y < 0 || y >= iconSize {
				continue
			}
			dx := float64(x) - cx
			dy := float64(y) - cy
			d := math.Hypot(dx, dy)
			if d > outerR || d < innerR {
				continue
			}
			ang := math.Atan2(dy, dx) // -Pi..Pi, 0 = east
			// Normalize relative to start into [0, 2Pi).
			rel := ang - start
			for rel < 0 {
				rel += 2 * math.Pi
			}
			for rel >= 2*math.Pi {
				rel -= 2 * math.Pi
			}
			if rel <= sweep {
				img.Set(x, y, c)
			}
		}
	}
}

func renderBatteryIcon(text string, percent int, fg color.RGBA) []byte {
	img := newIconCanvas()

	// Battery body and cap (horizontal).
	const (
		bx0, by0 = 3, 8
		bx1, by1 = 26, 24
		capX0    = 26
		capX1    = 29
		capY0    = 12
		capY1    = 20
		pad      = 2
	)

	// Outline body.
	for y := by0; y <= by1; y++ {
		for x := bx0; x <= bx1; x++ {
			edge := x == bx0 || x == bx1 || y == by0 || y == by1
			if edge {
				img.Set(x, y, batOutline)
			} else {
				img.Set(x, y, trackColor)
			}
		}
	}
	// Cap.
	for y := capY0; y <= capY1; y++ {
		for x := capX0; x <= capX1; x++ {
			img.Set(x, y, batOutline)
		}
	}

	// Fill from left inside the body.
	innerW := (bx1 - bx0 - 1) - pad*2
	fillW := percent * innerW / 100
	if fillW < 1 && percent > 0 {
		fillW = 1
	}
	if fillW > innerW {
		fillW = innerW
	}
	c := barColor(percent)
	for y := by0 + 1 + pad; y <= by1-1-pad; y++ {
		for x := 0; x < fillW; x++ {
			img.Set(bx0+1+pad+x, y, c)
		}
	}

	// Digits over the battery (scale 2 so they fit).
	drawCenteredText(img, text, fg, 2, 0)
	return encodeICO(img)
}

// renderSpinnerFrame draws one frame of a rotating arc spinner as a 32x32 ICO.
func renderSpinnerFrame(frame int) []byte {
	img := newIconCanvas()

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
