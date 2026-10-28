// Command ico writes a Windows icon (.ico) holding a PNG image at
// several sizes, made from one large PNG. scripts/package.sh uses it for
// the installer, shortcuts and Explorer's context menu entry.
//
//	go run ./scripts/ico assets/icon.png goplay.ico
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"

	"golang.org/x/image/draw"
)

var sizes = []int{256, 64, 48, 32, 24, 16}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: ico <in.png> <out.ico>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "ico:", err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		return err
	}

	var images [][]byte
	for _, s := range sizes {
		dst := image.NewNRGBA(image.Rect(0, 0, s, s))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
		var b bytes.Buffer
		if err := png.Encode(&b, dst); err != nil {
			return err
		}
		images = append(images, b.Bytes())
	}

	// ICONDIR, then one ICONDIRENTRY per image, then the PNG data.
	var b bytes.Buffer
	le := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	le([3]uint16{0, 1, uint16(len(images))})
	offset := 6 + 16*len(images)
	for i, s := range sizes {
		dim := uint8(s) // 256 is written as 0
		le([4]uint8{dim, dim, 0, 0})
		le([2]uint16{1, 32})
		le([2]uint32{uint32(len(images[i])), uint32(offset)})
		offset += len(images[i])
	}
	for _, img := range images {
		b.Write(img)
	}
	return os.WriteFile(out, b.Bytes(), 0o644)
}
