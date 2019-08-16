package imgur

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"
)

func TestUploadImage(t *testing.T) {
	img := image.NewRGBA(image.Rectangle{image.Point{0, 0}, image.Point{32, 32}})
	grey := color.RGBA{128, 128, 128, 0xff}
	for x := 0; x < 32; x++ {
		for y := 0; y < 32; y++ {
			switch {
			case x < 32/2 && y < 32/2: // upper left quadrant
				img.Set(x, y, color.Black)
			case x >= 32/2 && y >= 32/2: // lower right quadrant
				img.Set(x, y, color.White)
			default:
				img.Set(x, y, grey)
			}
		}
	}

	content := ByteContent{
		Type: "image/png",
	}
	buf := bytes.NewBuffer([]byte{})
	png.Encode(buf, img)
	content.Content = buf.Bytes()

	url, err := uploadImage(&content)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	dl, err := http.Get(url)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}
	defer dl.Body.Close()
	buf = &bytes.Buffer{}
	buf.ReadFrom(dl.Body)
	if bytes.Compare(buf.Bytes(), content.Content) != 0 {
		t.Log("image not match")
		t.FailNow()
	}
}
