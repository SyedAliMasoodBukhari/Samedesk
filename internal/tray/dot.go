package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
)

type tone int

const (
	grey tone = iota
	green
	blue
	red
)

var tones = map[tone]color.NRGBA{
	grey:  {142, 142, 147, 255},
	green: {52, 168, 83, 255},
	blue:  {65, 143, 204, 255}, // the SameDesk accent
	red:   {214, 69, 65, 255},
}

// dot draws the status light: a small antialiased circle, centred in a 32 px
// square (a 16 pt menu image on Retina).
func dot(t tone) []byte {
	const size, r = 32, 7.0
	c := tones[t]
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)+0.5-size/2, float64(y)+0.5-size/2)
			if a := math.Max(0, math.Min(1, r+0.5-d)); a > 0 {
				img.SetNRGBA(x, y, color.NRGBA{c.R, c.G, c.B, uint8(a * 255)})
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// pngToICO wraps a PNG in an .ico container, which Windows needs for menu images.
func pngToICO(p []byte) []byte {
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	w([3]uint16{0, 1, 1})                // reserved, type icon, one image
	w([4]uint8{32, 32, 0, 0})            // width, height, palette, reserved
	w([2]uint16{1, 32})                  // planes, bits per pixel
	w([2]uint32{uint32(len(p)), 6 + 16}) // size, offset
	b.Write(p)
	return b.Bytes()
}
