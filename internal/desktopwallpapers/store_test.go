package desktopwallpapers

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A 128×128 lossy WebP (simple format: a single "VP8 " chunk).
const testWebP = "UklGRuYBAABXRUJQVlA4INoBAACwEQCdASqAAIAAPjEYikKiIaEVDZ1EIAMEsYBpcxr/ldjJ7TTTfNtnz/oH+A9gGcA/gH96/" +
	"YzfQOAz6XDyK9WXDLQb6QvFILrj5wHXz3hKCZnqb0FI9B+oEIVtzxuGnsTfC9YoV5+CHdc99m22g0MQROjt4970E1RDO2q4j" +
	"M/TSPZ7wR7+MBq6loHdS7xvQR0SiUSiTwAA/v/YoqY+31u2qXGJyh1JBhg4zb8eGEcC/sjAEPn7/CgbEdPtO420cqGbn5RDDVr" +
	"vrW17RVnTXO9l6epXmTFmXrvAEWimiYz2VBDidhyCNz9yw2w1llKjDSDTI2tW+n8aFErnLjjPfvXEgc4IYKrfvoAAdAKtgEBvi" +
	"ofE83Kw9YRF7DnwMrChnNbGEL69zKHuN+v+Tf9QZiAkltjO0J5KnIr9q8HDv71ZZRudr/wd34qSAVOw49GY5WvOsDcFdwZGR7" +
	"ti+Nv6YZII3Ev+aZls77Nw41TO96Z5AETMPYDO5PHBPE1uyygCXjBheCDBg4KZCVxKDcLipucRbOhnjd4w3veauvC434fehjx/" +
	"GGvgnx/BCDtQc2bs45Hy9hjsJU7TbQNUzt3QJ1txwjy6Df3CoHc5BLqVj7iBXJRAAAEp/JgAAAA="

func webPFixture(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(testWebP)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func riffChunk(fourCC string, payload []byte) []byte {
	chunk := append([]byte(fourCC), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(chunk[4:8], uint32(len(payload)))
	chunk = append(chunk, payload...)
	if len(payload)%2 == 1 {
		chunk = append(chunk, 0)
	}
	return chunk
}

func riff(chunks ...[]byte) []byte {
	body := []byte("WEBP")
	for _, chunk := range chunks {
		body = append(body, chunk...)
	}
	out := append([]byte("RIFF"), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(body)))
	return append(out, body...)
}

// webPWithMetadata wraps the fixture's bitstream in an extended container that
// also carries EXIF and XMP chunks.
func webPWithMetadata(t *testing.T) []byte {
	simple := webPFixture(t)
	header := make([]byte, 10)
	header[0] = 0x08 | 0x04 // EXIF and XMP flags
	header[4], header[7] = 127, 127
	return riff(riffChunk("VP8X", header), simple[12:], riffChunk("EXIF", []byte("GPS 31.2,121.5")), riffChunk("XMP ", []byte("<x:xmpmeta/>")))
}

func jpegFixture(t *testing.T, width, height int, fill color.Color, extra ...[]byte) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, fill)
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	out := append([]byte{}, data[:2]...)
	for _, segment := range extra {
		out = append(out, segment...)
	}
	return append(out, data[2:]...)
}

func jpegSegment(marker byte, payload string) []byte {
	segment := []byte{0xff, marker, 0, 0}
	binary.BigEndian.PutUint16(segment[2:], uint16(len(payload)+2))
	return append(segment, payload...)
}

func testMeta() Metadata {
	return Metadata{Name: "山谷日落", FocusX: 700, FocusY: 320, Theme: &Theme{Brand: "#e8b86a", Neutral: "#1c3a4a", Signature: "#4f8fa8"}}
}

func mustPrepare(t *testing.T, meta Metadata, img, thumb []byte) *Prepared {
	t.Helper()
	prepared, err := Prepare(meta, img, thumb)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return prepared
}

func TestAddListFileAndDelete(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thumb := jpegFixture(t, 64, 36, color.RGBA{255, 255, 255, 255})
	added, err := store.Add(mustPrepare(t, testMeta(), webPFixture(t), thumb))
	if err != nil {
		t.Fatal(err)
	}
	if !ValidID(added.ID) || added.Format != "webp" || added.Width != 128 || added.Height != 128 || added.Name != "山谷日落" ||
		added.FocusX != 700 || added.Theme == nil || added.Theme.Brand != "#e8b86a" || len(added.ImageDigest) != 64 {
		t.Fatalf("unexpected wallpaper: %#v", added)
	}
	if added.Luminance < 95 {
		t.Fatalf("white thumbnail luminance = %d", added.Luminance)
	}
	list, usage := store.List()
	if len(list) != 1 || usage.Count != 1 || usage.Bytes != added.ImageBytes+added.ThumbBytes || usage.MaxCount != MaxWallpapers {
		t.Fatalf("list = %#v usage = %#v", list, usage)
	}
	file, size, contentType, err := store.OpenFile(added.ID, "image")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(file)
	_ = file.Close()
	if contentType != "image/webp" || size != int64(len(data)) || !bytes.Equal(data, webPFixture(t)) {
		t.Fatalf("image = %d bytes %q", len(data), contentType)
	}
	thumbFile, _, contentType, err := store.OpenFile(added.ID, "thumb")
	if err != nil || contentType != "image/jpeg" {
		t.Fatalf("thumb = %q %v", contentType, err)
	}
	_ = thumbFile.Close()
	if _, _, _, err := store.OpenFile(added.ID, "../index"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown kind = %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(store.filesDir, added.ID+".image"))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("image file mode = %v %v", info.Mode(), err)
		}
	}
	if err := store.Delete(added.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.OpenFile(added.ID, "image"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted image = %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.filesDir, added.ID+".image")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted file remains: %v", err)
	}
	if err := store.Delete(added.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}

func TestMetadataIsStrippedFromWebPAndJPEG(t *testing.T) {
	prepared := mustPrepare(t, testMeta(), webPWithMetadata(t), jpegFixture(t, 32, 32, color.Black))
	if bytes.Contains(prepared.image, []byte("EXIF")) || bytes.Contains(prepared.image, []byte("GPS")) || bytes.Contains(prepared.image, []byte("xmpmeta")) {
		t.Fatal("WebP metadata was kept")
	}
	if !bytes.Contains(prepared.image, []byte("VP8X")) || prepared.image[20]&(0x20|0x08|0x04|0x02) != 0 {
		t.Fatalf("VP8X flags not cleared: %x", prepared.image[20])
	}

	withExif := jpegFixture(t, 96, 96, color.Gray{128}, jpegSegment(0xe1, "Exif\x00\x00GPS 31.2,121.5"), jpegSegment(0xfe, "camera comment"), jpegSegment(0xe2, "ICC_PROFILE"))
	prepared = mustPrepare(t, testMeta(), withExif, jpegFixture(t, 32, 32, color.Black))
	if prepared.format != "jpeg" || bytes.Contains(prepared.image, []byte("Exif")) || bytes.Contains(prepared.image, []byte("camera comment")) || bytes.Contains(prepared.image, []byte("ICC_PROFILE")) {
		t.Fatal("JPEG metadata was kept")
	}
	if _, err := jpeg.Decode(bytes.NewReader(prepared.image)); err != nil {
		t.Fatalf("stripped JPEG does not decode: %v", err)
	}
}

func TestPrepareRejectsUnsafeOrUnusableImages(t *testing.T) {
	thumb := jpegFixture(t, 32, 32, color.Black)
	simple := webPFixture(t)
	animated := riff(riffChunk("VP8X", append([]byte{0x02}, make([]byte, 9)...)), riffChunk("ANIM", make([]byte, 6)), simple[12:])
	trailing := append(append([]byte{}, simple...), []byte("<script>")...)
	jpegTrailing := append(jpegFixture(t, 96, 96, color.White), []byte("PK\x03\x04")...)
	cases := map[string][]byte{
		"empty":         nil,
		"svg":           []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
		"png":           []byte("\x89PNG\r\n\x1a\n"),
		"animated":      animated,
		"trailing webp": trailing,
		"trailing jpeg": jpegTrailing,
		"truncated":     simple[:len(simple)-40],
		"too small":     jpegFixture(t, 48, 48, color.White),
		"too large":     bytes.Repeat([]byte{0xff}, MaxImageBytes+1),
	}
	for name, data := range cases {
		if _, err := Prepare(testMeta(), data, thumb); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := Prepare(testMeta(), simple, jpegFixture(t, 700, 32, color.Black)); !errors.Is(err, ErrInvalid) {
		t.Errorf("oversized thumbnail accepted: %v", err)
	}
}

func TestPrepareValidatesMetadata(t *testing.T) {
	img, thumb := webPFixture(t), jpegFixture(t, 32, 32, color.Black)
	cases := map[string]Metadata{
		"empty name":   {Name: "  "},
		"long name":    {Name: strings.Repeat("长", MaxNameRunes+1)},
		"control name": {Name: "a\u0007b"},
		"focus":        {Name: "a", FocusX: FocusScale + 1},
		"theme":        {Name: "a", Theme: &Theme{Brand: "red", Neutral: "#000000", Signature: "#ffffff"}},
		"theme case":   {Name: "a", Theme: &Theme{Brand: "#FFFFFF", Neutral: "#000000", Signature: "#ffffff"}},
	}
	for name, meta := range cases {
		var validation *ValidationError
		if _, err := Prepare(meta, img, thumb); !errors.As(err, &validation) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	prepared := mustPrepare(t, Metadata{Name: "  海边  "}, img, thumb)
	if prepared.meta.Name != "海边" || prepared.meta.Theme != nil {
		t.Fatalf("normalized meta = %#v", prepared.meta)
	}
}

func TestQuotaLimitsCountAndBytes(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prepared := mustPrepare(t, testMeta(), webPFixture(t), jpegFixture(t, 32, 32, color.Black))
	for range MaxWallpapers {
		if _, err := store.Add(prepared); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Add(prepared); !errors.Is(err, ErrQuota) {
		t.Fatalf("thirteenth wallpaper = %v", err)
	}

	store, err = Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	large := *prepared
	large.image = make([]byte, MaxTotalBytes)
	if _, err := store.Add(&large); !errors.Is(err, ErrQuota) {
		t.Fatalf("byte quota = %v", err)
	}
}

func TestOpenRepairsIndexAndRemovesStrayFiles(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared := mustPrepare(t, testMeta(), webPFixture(t), jpegFixture(t, 32, 32, color.Black))
	kept, err := store.Add(prepared)
	if err != nil {
		t.Fatal(err)
	}
	lost, err := store.Add(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "files", lost.ID+".thumb")); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(root, "files", ".desktop-wallpaper-123")
	if err := os.WriteFile(stray, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := reopened.List()
	if len(list) != 1 || list[0].ID != kept.ID {
		t.Fatalf("reopened list = %#v", list)
	}
	for _, path := range []string{stray, filepath.Join(root, "files", lost.ID+".image")} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s was not removed: %v", path, err)
		}
	}

	for name, content := range map[string][]byte{"unparsable": []byte("{not json"), "empty": {}} {
		if err := os.WriteFile(filepath.Join(root, "index.json"), content, 0o600); err != nil {
			t.Fatal(err)
		}
		reopened, err = Open(root)
		if err != nil {
			t.Fatalf("%s index blocks startup: %v", name, err)
		}
		if list, _ := reopened.List(); len(list) != 0 {
			t.Fatalf("%s index list = %#v", name, list)
		}
	}
}
