package ai

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// Keep the encoded body outside the timed region: history reads an existing row.
func BenchmarkAttachmentMetadataLarge(b *testing.B) {
	body := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x73}, 4<<20))
	data := []byte(`[{"name":"first.png","mimeType":"image/png","kind":"image","data":"` + body + `"},{"name":"second.png","mimeType":"image/png","kind":"image","data":"` + body + `"}]`)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for b.Loop() {
		items, err := decodeAttachmentMetadata(data)
		if err != nil || len(items) != 2 || items[0].Size != 4<<20 || items[1].Size != 4<<20 {
			b.Fatalf("metadata: %#v / %v", items, err)
		}
	}
}
