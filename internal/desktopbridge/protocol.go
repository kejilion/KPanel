// Package desktopbridge implements the bounded RDCleanPath adapter for the
// Windows machine's own RDP listener. It never accepts a forwarding destination.
package desktopbridge

import (
	"bytes"
	"crypto/subtle"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"io"
)

const (
	maxHandshakeBytes = 64 << 10
	maxX224Bytes      = 260 // TPKT header plus the one-byte X.224 length indicator.
	protocolHybrid    = uint32(2)
	protocolHybridEx  = uint32(8)
)

type request struct {
	x224      []byte
	protocols uint32
}

// readDER reads exactly one PDU, preserving subsequent CredSSP bytes. DER must
// use definite, minimal lengths; the bound is checked before any allocation.
func readDER(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:2]); err != nil {
		return nil, err
	}
	if header[0] != 0x30 {
		return nil, ErrRequest
	}
	n, size := 2, int(header[1])
	if size >= 128 {
		count := size & 127
		if count < 1 || count > 2 {
			return nil, ErrRequest
		}
		if _, err := io.ReadFull(r, header[2:2+count]); err != nil {
			return nil, err
		}
		if header[2] == 0 {
			return nil, ErrRequest
		}
		size = 0
		for _, b := range header[2 : 2+count] {
			size = size<<8 | int(b)
		}
		if size < 128 {
			return nil, ErrRequest
		}
		n += count
	}
	if size+n > maxHandshakeBytes {
		return nil, ErrRequest
	}
	data := make([]byte, n+size)
	copy(data, header[:n])
	if _, err := io.ReadFull(r, data[n:]); err != nil {
		return nil, err
	}
	return data, nil
}

func decodeRequest(data []byte, nonce string) (request, error) {
	decodedNonce, err := hex.DecodeString(nonce)
	if err != nil || len(decodedNonce) != 32 || len(nonce) != 64 {
		return request{}, ErrAuthentication
	}
	if len(data) > maxHandshakeBytes {
		return request{}, ErrRequest
	}
	var outer asn1.RawValue
	rest, err := asn1.Unmarshal(data, &outer)
	if err != nil || len(rest) != 0 || outer.Class != 0 || outer.Tag != 16 || !outer.IsCompound {
		return request{}, ErrRequest
	}
	body := outer.Bytes
	var result request
	// Only these four fields, in schema order, are permitted. In particular PCB,
	// serverAuth, duplicate, unknown and response-only fields are rejected.
	for _, tag := range []int{0, 2, 3, 6} {
		var field, value asn1.RawValue
		body, err = asn1.Unmarshal(body, &field)
		if err != nil || field.Class != 2 || field.Tag != tag || !field.IsCompound {
			return request{}, ErrRequest
		}
		extra, err := asn1.Unmarshal(field.Bytes, &value)
		if err != nil || len(extra) != 0 || value.Class != 0 || value.IsCompound {
			return request{}, ErrRequest
		}
		switch tag {
		case 0:
			if value.Tag != 2 || !bytes.Equal(value.Bytes, []byte{0x0d, 0x3e}) {
				return request{}, ErrRequest
			}
		case 2:
			if value.Tag != 12 || string(value.Bytes) != "localhost" {
				return request{}, ErrDestination
			}
		case 3:
			if value.Tag != 12 || subtle.ConstantTimeCompare(value.Bytes, []byte(nonce)) != 1 {
				return request{}, ErrAuthentication
			}
		case 6:
			if value.Tag != 4 {
				return request{}, ErrRequest
			}
			result.x224 = append([]byte(nil), value.Bytes...)
		}
	}
	if len(body) != 0 {
		return request{}, ErrRequest
	}
	result.protocols, err = validateX224Request(result.x224)
	if err != nil {
		return request{}, err
	}
	return result, nil
}

func validTPKT(p []byte, code byte) bool {
	return len(p) >= 11 && len(p) <= maxX224Bytes && p[0] == 3 && p[1] == 0 &&
		int(binary.BigEndian.Uint16(p[2:4])) == len(p) && int(p[4])+5 == len(p) && p[5] == code && p[10] == 0
}

func validateX224Request(p []byte) (uint32, error) {
	if !validTPKT(p, 0xe0) {
		return 0, ErrNegotiation
	}
	payload := p[11:]
	if bytes.HasPrefix(payload, []byte("Cookie: ")) {
		end := bytes.Index(payload, []byte("\r\n"))
		if end < 0 {
			return 0, ErrNegotiation
		}
		payload = payload[end+2:]
	}
	if len(payload) < 8 || payload[0] != 1 || payload[1] & ^byte(0x0b) != 0 || binary.LittleEndian.Uint16(payload[2:4]) != 8 {
		return 0, ErrNegotiation
	}
	if payload[1]&8 == 0 {
		if len(payload) != 8 {
			return 0, ErrNegotiation
		}
	} else {
		// RDP_NEG_CORRELATION_INFO follows RDP_NEG_REQ when flag 0x08 is set.
		if len(payload) != 44 || payload[8] != 6 || payload[9] != 0 || binary.LittleEndian.Uint16(payload[10:12]) != 36 || !bytes.Equal(payload[28:44], make([]byte, 16)) {
			return 0, ErrNegotiation
		}
	}
	protocols := binary.LittleEndian.Uint32(payload[4:8])
	if protocols & ^uint32(0x0f) != 0 || protocols&(protocolHybrid|protocolHybridEx) == 0 {
		return 0, ErrNLARequired
	}
	return protocols, nil
}

func readX224Response(r io.Reader, offered uint32) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint16(header[2:]))
	if header[0] != 3 || header[1] != 0 || size != 19 {
		return nil, ErrNegotiation
	}
	p := make([]byte, size)
	copy(p, header[:])
	if _, err := io.ReadFull(r, p[4:]); err != nil {
		return nil, err
	}
	if !validTPKT(p, 0xd0) || p[11] != 2 || binary.LittleEndian.Uint16(p[13:15]) != 8 {
		return nil, ErrNegotiation
	}
	selected := binary.LittleEndian.Uint32(p[15:19])
	if (selected != protocolHybrid && selected != protocolHybridEx) || offered&selected == 0 {
		return nil, ErrNLARequired
	}
	return p, nil
}

func encodeResponse(x224 []byte, certificates [][]byte) ([]byte, error) {
	value := struct {
		Version      int      `asn1:"explicit,tag:0"`
		X224         []byte   `asn1:"explicit,tag:6"`
		Certificates [][]byte `asn1:"explicit,tag:7"`
		Address      string   `asn1:"utf8,explicit,tag:9"`
	}{3390, x224, certificates, "127.0.0.1"}
	data, err := asn1.Marshal(value)
	if err != nil || len(data) > maxHandshakeBytes {
		return nil, ErrCertificate
	}
	return data, nil
}

func encodeFailure(status int) []byte {
	type failure struct {
		Code int `asn1:"explicit,tag:0"`
		HTTP int `asn1:"explicit,tag:1"`
	}
	data, _ := asn1.Marshal(struct {
		Version int     `asn1:"explicit,tag:0"`
		Error   failure `asn1:"explicit,tag:1"`
	}{3390, failure{1, status}})
	return data
}
