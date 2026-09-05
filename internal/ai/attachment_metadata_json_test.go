package ai

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// Keep the previous metadata decoder as a differential oracle. In particular,
// duplicate fields, null and streaming Base64 errors are part of its behavior.
func previousAttachmentMetadata(data []byte) ([]Attachment, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) > maxAttachmentReadBytes {
		return nil, errors.New("row limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if token != nil && token != json.Delim('[') {
		return nil, errors.New("expected array")
	}
	items := []Attachment{}
	for token != nil && decoder.More() {
		if len(items) >= 4 {
			return nil, errors.New("item limit")
		}
		var item storedAttachment
		if err := decoder.Decode(&item); err != nil {
			return nil, err
		}
		size, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding, strings.NewReader(item.Data)))
		if err != nil {
			return nil, err
		}
		items = append(items, Attachment{Name: item.Name, MimeType: item.MimeType, Kind: item.Kind, Size: int(size)})
	}
	if token != nil {
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("trailing data")
	}
	return items, nil
}

func compareAttachmentMetadata(t *testing.T, data []byte) {
	t.Helper()
	want, oldErr := previousAttachmentMetadata(data)
	got, err := decodeAttachmentMetadata(data)
	if (err != nil) != (oldErr != nil) || (err == nil && !reflect.DeepEqual(got, want)) {
		t.Fatalf("input %.160q (len %d): new=%#v/%v old=%#v/%v", data, len(data), got, err, want, oldErr)
	}
}

func TestAttachmentMetadataJSONEquivalence(t *testing.T) {
	cases := []string{
		"", " ", "[]", "null", " [null,{},null,{}] ", "[null,null,null,null,null]",
		"[{},{},{},{},{}]", "[1]", "[[]]", "[true]", "[\"Zg==\"]", "{}", "0", "true",
		`[{"data":"Zg=="}]`, `[{"data":null}]`, `[{}]`, `[{"data":"Zg==","data":null}]`,
		`[{"data":"%%%","data":"Zg=="}]`, `[{"data":"Zg==","data":"%%%"}]`,
		`[{"data":1,"data":"Zg=="}]`, `[{"data":"Zg==","data":false}]`,
		`[{"dAtA":"Zg==","DATA":"Zm8=","name":"first","NAME":null}]`,
		`[{"d\u0061ta":"\u005a\u0067\u003d\u003D","name":"\u6587\u4ef6"}]`,
		`[{"data":"\/w=="}]`, `[{"data":"\u002fw=="}]`, `[{"data":"\u002bw=="}]`,
		`[{"data":"Z\rm\n8="}]`, `[{"data":"Z\u000dm\u000a8="}]`,
		`[{"data":"Zg==\r\n"}]`, `[{"data":"Zg==\u000d\u000a"}]`,
		`[{"data":"\u00ff"}]`, `[{"data":"\ud800"}]`, `[{"data":"\ud83d\ude00"}]`,
		`[{"data":"\\"}]`, `[{"data":"\""}]`, `[{"data":"\b\f\t"}]`,
		`[{"name":null,"kind":"text","mimeType":"text/plain","unknown":{"data":"%%%"},"data":"Zg=="}]`,
		`[{"name":1,"name":"later","data":""}]`,
		`[{"data":"Zg=="},]`, `[{"data":"\x41"}]`, `[{"data":"\u123"}]`,
		`[{"data":"Zg=="}] []`, `null null`, `[{"data":"Zg=="}`, `[`,
	}
	for i, data := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) { compareAttachmentMetadata(t, []byte(data)) })
	}
	// All ASCII values through JSON Unicode escapes, including the entire
	// Base64 alphabet and controls; invalid UTF-8 is rejected by Base64 too.
	for b := 0; b < 256; b++ {
		compareAttachmentMetadata(t, []byte(fmt.Sprintf(`[{"data":"\u%04xAAA"}]`, b)))
		encoded, _ := json.Marshal(string([]byte{byte(b), 'A', 'A', 'A'}))
		compareAttachmentMetadata(t, append(append([]byte(`[{"data":`), encoded...), '}', ']'))
	}
	// Cross the Base64 reader's internal input boundaries with escapes and
	// padding. Duplicate field values must only validate their last value.
	for _, n := range []int{1, 2, 3, 255, 256, 257, 1023, 1024, 1025} {
		body := strings.Repeat("Zm9v", n)
		for _, tail := range []string{"", "Zg==", "Zg===", "Zg==Zg==", `Zg==\r\n`, `\u005ag==`, `\u00ff`} {
			compareAttachmentMetadata(t, []byte(`[{"data":"`+body+tail+`"}]`))
		}
	}
}

func TestAttachmentMetadataJSONLimits(t *testing.T) {
	for _, n := range []int{4, 5, 100000} {
		data := []byte("[" + strings.Repeat("null,", n-1) + "null]")
		compareAttachmentMetadata(t, data)
		_, err := decodeAttachmentMetadata(data)
		if (err != nil) != (n > 4) {
			t.Fatalf("count %d error=%v", n, err)
		}
	}
	for _, n := range []int{maxAttachmentReadBytes - 1, maxAttachmentReadBytes, maxAttachmentReadBytes + 1} {
		data := append([]byte("[]"), bytes.Repeat([]byte(" "), n-2)...)
		compareAttachmentMetadata(t, data)
		_, err := decodeAttachmentMetadata(data)
		if (err != nil) != (n > maxAttachmentReadBytes) {
			t.Fatalf("length %d error=%v", n, err)
		}
	}
}

func TestAttachmentMetadataJSONDepthBoundary(t *testing.T) {
	// encodeAttachments writes flat string fields. Unknown legacy nesting is
	// still parsed, but Unmarshal counts the outer array toward its depth limit;
	// the previous per-item Decoder did not. Keep this explicit failure boundary.
	for _, depth := range []int{9998, 9999, 10000} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			data := []byte(`[{"data":"Zg==","unknown":` + strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth) + `}]`)
			original := bytes.Clone(data)
			old, oldErr := previousAttachmentMetadata(data)
			got, err := decodeAttachmentMetadata(data)
			if !bytes.Equal(data, original) {
				t.Fatal("reading metadata changed the stored JSON")
			}
			if depth < 10000 {
				if oldErr != nil || len(old) != 1 || old[0].Size != 1 {
					t.Fatalf("previous decoder boundary changed: items=%#v err=%v", old, oldErr)
				}
			} else if oldErr == nil {
				t.Fatal("previous decoder unexpectedly accepted excessive depth")
			}
			if depth == 9998 {
				if err != nil || !reflect.DeepEqual(got, old) {
					t.Fatalf("supported depth: items=%#v err=%v", got, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "exceeded max depth") || got != nil {
				t.Fatalf("expected explicit depth failure without partial success: items=%#v err=%v", got, err)
			}
		})
	}
}

func TestAttachmentMetadataDoesNotCopyEncodedBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"small", []byte(`[{"data":"Zm9v"}]`)},
		{"two_large", []byte(`[{"data":"` + strings.Repeat("Zm9v", 1393617) + `"},{"data":"` + strings.Repeat("Zm9v", 1393617) + `"}]`)},
		{"escaped_large", []byte(`[{"data":"` + strings.Repeat(`\u005a\u006d\u0039\u0076`, 349525) + `"}]`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compareAttachmentMetadata(t, tc.data)
			for _, implementation := range []struct {
				name string
				read func([]byte) ([]Attachment, error)
			}{{"previous", previousAttachmentMetadata}, {"current", decodeAttachmentMetadata}} {
				// Warm reflection/JSON metadata and the discard buffer before counting
				// allocated bytes. No forced GC or memory-limit tuning is used.
				if _, err := implementation.read(tc.data); err != nil {
					t.Fatal(err)
				}
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				const iterations = 3
				for i := 0; i < iterations; i++ {
					items, err := implementation.read(tc.data)
					if err != nil || len(items) == 0 {
						t.Fatalf("read failed: %v", err)
					}
				}
				runtime.ReadMemStats(&after)
				allocated := (after.TotalAlloc - before.TotalAlloc) / iterations
				t.Logf("%s inputBytes=%d allocatedBytesPerRead=%d", implementation.name, len(tc.data), allocated)
				if implementation.name == "current" && allocated > 256<<10 {
					t.Fatalf("metadata parsing allocated %d bytes per read; encoded body must remain borrowed", allocated)
				}
			}
		})
	}
}

func FuzzAttachmentMetadataJSONEquivalence(f *testing.F) {
	for _, data := range []string{`[{"data":"Zg=="}]`, `[null,{}]`, `[{"data":"%%%","DATA":"\u005ag=="}]`, `[{"data":"Z\rm\n8="}]`} {
		f.Add([]byte(data))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		// The separate depth test freezes the one legacy difference above this
		// input size; fuzz covers ordinary accepted JSON and corrupt records.
		if len(data) > 16<<10 {
			t.Skip()
		}
		compareAttachmentMetadata(t, data)
	})
}
