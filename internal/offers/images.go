package offers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"path"

	"golang.org/x/image/webp"
)

var ErrImageInvalid = errors.New("offers image is invalid")

// imageKind is the banner slot an image is pinned to.
type imageKind int

const (
	cardImage imageKind = iota
	wideImage
)

func (kind imageKind) limits() (width, height int, maxBytes int64) {
	if kind == wideImage {
		return WideWidth, WideHeight, MaxWideBytes
	}
	return CardWidth, CardHeight, MaxCardBytes
}

// sniffContentType recognises only the three static formats the manifest
// may reference, from the file signature rather than any server header.
func sniffContentType(data []byte) string {
	switch {
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp"
	case len(data) >= 8 && bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "image/jpeg"
	default:
		return ""
	}
}

func extensionContentType(name string) string {
	switch path.Ext(name) {
	case ".webp":
		return "image/webp"
	case ".png":
		return "image/png"
	case ".jpg":
		return "image/jpeg"
	default:
		return ""
	}
}

// validateImage checks the pinned digest, the declared format, the exact slot
// dimensions and that the whole file decodes, so a truncated or oversized
// banner never reaches a browser.
func validateImage(data []byte, pinned Image, kind imageKind) (string, error) {
	width, height, maxBytes := kind.limits()
	if len(data) == 0 || int64(len(data)) > maxBytes {
		return "", ErrImageInvalid
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != pinned.SHA256 {
		return "", ErrImageInvalid
	}
	contentType := sniffContentType(data)
	if contentType == "" || contentType != extensionContentType(pinned.Path) {
		return "", ErrImageInvalid
	}
	config, err := decodeConfig(data, contentType)
	if err != nil || config.Width != width || config.Height != height {
		return "", ErrImageInvalid
	}
	if err := decodeFull(data, contentType); err != nil {
		return "", ErrImageInvalid
	}
	return contentType, nil
}

func decodeConfig(data []byte, contentType string) (image.Config, error) {
	reader := bytes.NewReader(data)
	switch contentType {
	case "image/webp":
		return webp.DecodeConfig(reader)
	case "image/png":
		return png.DecodeConfig(reader)
	case "image/jpeg":
		return jpeg.DecodeConfig(reader)
	default:
		return image.Config{}, ErrImageInvalid
	}
}

func decodeFull(data []byte, contentType string) error {
	reader := bytes.NewReader(data)
	var err error
	switch contentType {
	case "image/webp":
		_, err = webp.Decode(reader)
	case "image/png":
		_, err = png.Decode(reader)
	case "image/jpeg":
		_, err = jpeg.Decode(reader)
	default:
		err = ErrImageInvalid
	}
	return err
}
