package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// RobotIcon generates a 22x22 robot template icon for the macOS menu bar.
// Template images should be black on transparent — macOS handles light/dark mode.
func RobotIcon() []byte {
	const size = 22
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	black := color.RGBA{0, 0, 0, 255}

	set := func(x, y int) {
		if x >= 0 && x < size && y >= 0 && y < size {
			img.Set(x, y, black)
		}
	}

	// Antenna
	set(11, 1)
	set(11, 2)
	set(10, 2)
	set(12, 2)

	// Head (8x6 box, from x=7 to x=14, y=3 to y=8)
	for x := 7; x <= 14; x++ {
		set(x, 3)
		set(x, 8)
	}
	for y := 3; y <= 8; y++ {
		set(7, y)
		set(14, y)
	}

	// Eyes (2x2 each)
	set(9, 5)
	set(10, 5)
	set(9, 6)
	set(10, 6)

	set(12, 5)
	set(13, 5)
	set(12, 6)
	set(13, 6)

	// Mouth
	set(9, 7)
	set(10, 7)
	set(11, 7)
	set(12, 7)

	// Neck
	set(10, 9)
	set(11, 9)

	// Body (10x6 box, from x=6 to x=15, y=10 to y=15)
	for x := 6; x <= 15; x++ {
		set(x, 10)
		set(x, 15)
	}
	for y := 10; y <= 15; y++ {
		set(6, y)
		set(15, y)
	}

	// Body detail — chest panel
	for x := 9; x <= 12; x++ {
		set(x, 12)
		set(x, 13)
	}
	// Chest light
	set(10, 12)
	set(11, 12)

	// Arms
	for y := 11; y <= 14; y++ {
		set(4, y)
		set(5, y)
		set(16, y)
		set(17, y)
	}

	// Legs
	for y := 16; y <= 19; y++ {
		set(8, y)
		set(9, y)
		set(12, y)
		set(13, y)
	}

	// Feet
	set(7, 19)
	set(8, 19)
	set(9, 19)
	set(12, 19)
	set(13, 19)
	set(14, 19)

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
