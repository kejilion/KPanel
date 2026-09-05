package ai

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
)

// These views live only while the caller owns the current attachments_json row.
// json.Unmarshal validates JSON without Decoder's growing object buffer. Data
// deliberately borrows its token: RawMessage and string would copy the body.
type attachmentMetadataJSON struct {
	Name     string             `json:"name"`
	MimeType string             `json:"mimeType"`
	Kind     string             `json:"kind"`
	Data     attachmentDataJSON `json:"data"`
	present  bool
	overflow bool
}

func (item *attachmentMetadataJSON) UnmarshalJSON(data []byte) error {
	if item.overflow {
		return errors.New("message attachment record exceeds the item read limit")
	}
	item.present = true
	type fields attachmentMetadataJSON
	return json.Unmarshal(data, (*fields)(item))
}

type attachmentDataJSON []byte

func (data *attachmentDataJSON) UnmarshalJSON(token []byte) error {
	// Match a string field: null leaves a preceding duplicate value unchanged.
	if bytes.Equal(token, []byte("null")) {
		return nil
	}
	if len(token) < 2 || token[0] != '"' {
		return errors.New("attachment data must be a JSON string")
	}
	*data = token[1 : len(token)-1]
	return nil
}

func (data attachmentDataJSON) decodedSize() (int, error) {
	// Validate only the final duplicate value, just as storedAttachment.Data did.
	// Neither the unescaped Base64 string nor its decoded body is materialized.
	size, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding, &attachmentJSONReader{data: data}))
	return int(size), err
}

// attachmentJSONReader unescapes an already validated JSON string for the
// Base64 decoder. Base64 only accepts ASCII, so non-ASCII Unicode escapes can
// immediately supply an invalid byte; no general Unicode decoder is needed.
type attachmentJSONReader struct {
	data []byte
}

func (r *attachmentJSONReader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) && len(r.data) > 0 {
		chunk := r.data[:min(len(r.data), len(p)-n)]
		plain := bytes.IndexByte(chunk, '\\')
		if plain < 0 {
			plain = len(chunk)
		}
		n += copy(p[n:], chunk[:plain])
		r.data = r.data[plain:]
		if n == len(p) || len(r.data) == 0 {
			break
		}
		// JSON validation guarantees a complete, valid escape at this position.
		b := r.data[1]
		r.data = r.data[2:]
		switch b {
		case 'b':
			b = '\b'
		case 'f':
			b = '\f'
		case 'n':
			b = '\n'
		case 'r':
			b = '\r'
		case 't':
			b = '\t'
		case 'u':
			var code [2]byte
			_, _ = hex.Decode(code[:], r.data[:4])
			r.data = r.data[4:]
			b = code[1]
			if code[0] != 0 || b >= 0x80 {
				b = 0xff // Every non-ASCII code point is invalid Base64.
			}
		}
		p[n] = b
		n++
	}
	if n == 0 && len(p) > 0 {
		return 0, io.EOF
	}
	return n, nil
}
