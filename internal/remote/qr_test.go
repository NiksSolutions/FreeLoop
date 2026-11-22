package remote

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"

	"rsc.io/qr/coding"
	"rsc.io/qr/gf256"
)

// GUI-052, issue 031: the QR code on the Phone page decodes to the link shown
// beside it, read back from the PNG the page gets.
func TestPairingQRHoldsTheLink(t *testing.T) {
	for _, link := range []string{
		"https://phone.test:8443/phone/#pair=ABCD-EFGH-JKMN-PQRS-TVWX",
		"https://macbook-pro.tail1234ab.ts.net:8443/phone/#pair=0123-4567-89AB-CDEF-GHJK",
	} {
		url, err := PairingQR(link)
		if err != nil {
			t.Fatal(err)
		}
		data, ok := strings.CutPrefix(url, "data:image/png;base64,")
		if !ok {
			t.Fatalf("not a PNG data URL: %.40s", url)
		}
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if got := readQR(t, img); got != link {
			t.Errorf("the QR code says %q, the page shows %q", got, link)
		}
	}
	if _, err := PairingQR(""); err == nil {
		t.Error("an empty link was drawn")
	}
	if _, err := PairingQR(strings.Repeat("a", maxQRText+1)); err == nil {
		t.Error("a link past the limit was drawn")
	}
}

// readQR decodes a QR code drawn on a white quiet zone: it samples the modules,
// finds the level and mask whose fixed patterns and format bits the image carries,
// reads the bytes, checks each block's Reed-Solomon bytes and returns the text of
// a byte-mode segment.
func readQR(t *testing.T, img image.Image) string {
	t.Helper()
	b := img.Bounds()
	dark := func(x, y int) bool {
		r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
		return r+g+bl < 3*0x8000
	}
	// The finder's top row: the first dark run on the diagonal's row is seven
	// modules wide.
	edge := 0
	for edge < b.Dx() && !dark(edge, edge) {
		edge++
	}
	run := 0
	for edge+run < b.Dx() && dark(edge+run, edge) {
		run++
	}
	if edge == 0 || run == 0 || run%7 != 0 {
		t.Fatalf("no finder pattern: edge %d, run %d", edge, run)
	}
	scale := run / 7
	if edge < 4*scale {
		t.Fatalf("the quiet zone is %d px, a camera needs %d", edge, 4*scale)
	}
	size := (b.Dx() - 2*edge) / scale
	if (size-17)%4 != 0 || size < 21 {
		t.Fatalf("%d modules across is no QR size", size)
	}
	grid := make([][]bool, size)
	for y := range grid {
		grid[y] = make([]bool, size)
		for x := range grid[y] {
			grid[y][x] = dark(edge+x*scale+scale/2, edge+y*scale+scale/2)
		}
	}
	version := coding.Version((size - 17) / 4)
	var plan *coding.Plan
	for level := coding.L; level <= coding.H && plan == nil; level++ {
		for mask := coding.Mask(0); mask < 8; mask++ {
			p, err := coding.NewPlan(version, level, mask)
			if err != nil {
				t.Fatal(err)
			}
			if fixedMatch(p, grid) {
				plan = p
				break
			}
		}
	}
	if plan == nil {
		t.Fatal("no level and mask match the code's fixed patterns")
	}
	all := make([]byte, plan.DataBytes+plan.CheckBytes)
	for y, row := range plan.Pixel {
		for x, pix := range row {
			if r := pix.Role(); r != coding.Data && r != coding.Check {
				continue
			}
			if grid[y][x] != (pix&coding.Black != 0) {
				o := pix.Offset()
				all[o/8] |= 1 << (7 - o%8)
			}
		}
	}
	data, check := all[:plan.DataBytes], all[plan.DataBytes:]
	per := plan.CheckBytes / plan.Blocks
	rs := gf256.NewRSEncoder(coding.Field, per)
	short, long := plan.DataBytes/plan.Blocks, plan.Blocks-plan.DataBytes%plan.Blocks
	want := make([]byte, per)
	for i, rest := 0, data; i < plan.Blocks; i++ {
		n := short
		if i >= long {
			n++
		}
		rs.ECC(rest[:n], want)
		if !bytes.Equal(want, check[i*per:(i+1)*per]) {
			t.Fatalf("block %d's check bytes do not match its data", i)
		}
		rest = rest[n:]
	}
	if mode := data[0] >> 4; mode != 0b0100 {
		t.Fatalf("mode %04b, not bytes", mode)
	}
	if version > 9 {
		t.Fatalf("version %d needs a 16-bit count", version)
	}
	n := int(data[0]&0x0f)<<4 | int(data[1]>>4)
	out := make([]byte, n)
	for i := range out {
		out[i] = data[1+i]<<4 | data[2+i]>>4
	}
	return string(out)
}

// fixedMatch says whether every module the plan fixes (finders, timing,
// alignment, format and version bits, and the remainder) is as the image draws it.
func fixedMatch(p *coding.Plan, grid [][]bool) bool {
	for y, row := range p.Pixel {
		for x, pix := range row {
			if r := pix.Role(); r == coding.Data || r == coding.Check {
				continue
			}
			if grid[y][x] != (pix&coding.Black != 0) {
				return false
			}
		}
	}
	return true
}
