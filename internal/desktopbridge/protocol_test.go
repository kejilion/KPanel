package desktopbridge

import (
	"bytes"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
)

const testNonce = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var testX224 = []byte{3, 0, 0, 19, 14, 0xe0, 0, 0, 0, 0, 0, 1, 0, 8, 0, 3, 0, 0, 0}

func requestDER(t *testing.T, destination, nonce string, x224 []byte) []byte {
	t.Helper()
	data, err := asn1.Marshal(struct {
		Version     int    `asn1:"explicit,tag:0"`
		Destination string `asn1:"utf8,explicit,tag:2"`
		Nonce       string `asn1:"utf8,explicit,tag:3"`
		X224        []byte `asn1:"explicit,tag:6"`
	}{3390, destination, nonce, x224})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRDCleanPathGoldenDER(t *testing.T) {
	// Explicit tags and version are frozen by IronRDP RDCleanPath v1. These
	// literal vectors do not use this package's encoders to construct expectations.
	golden := unhex(t, "306ea00402020d3ea20b0c096c6f63616c686f7374a3420c40"+strings.Repeat("61", 64)+"a6150413030000130ee000000000000100080003000000")
	if !bytes.Equal(requestDER(t, "localhost", testNonce, testX224), golden) {
		t.Fatal("request differs from wire fixture")
	}
	got, err := decodeRequest(golden, testNonce)
	if err != nil || got.protocols != 3 || !bytes.Equal(got.x224, testX224) {
		t.Fatalf("request: %#v %v", got, err)
	}
	cert := []byte{0xde, 0xad, 0xbe, 0xff}
	response, err := encodeResponse(cert, [][]byte{cert, cert, cert})
	want := unhex(t, "3031a00402020d3ea6060404deadbeffa71430120404deadbeff0404deadbeff0404deadbeffa90b0c093132372e302e302e31")
	if err != nil || !bytes.Equal(response, want) {
		t.Fatalf("response %x %v", response, err)
	}
	if !bytes.Equal(encodeFailure(500), unhex(t, "3015a00402020d3ea10d300ba003020101a104020201f4")) {
		t.Fatal("error wire fixture differs")
	}
}

func TestRDCleanPathRejectsDestinationsAndNonce(t *testing.T) {
	for _, dest := range []string{"127.0.0.1", "localhost:3389", "LOCALHOST", "localhost.", "[::1]", "192.168.1.1", "example.com", "\\\\server\\share", "localhost\x00", ""} {
		if _, err := decodeRequest(requestDER(t, dest, testNonce, testX224), testNonce); !errors.Is(err, ErrDestination) {
			t.Fatalf("accepted %q: %v", dest, err)
		}
	}
	if _, err := decodeRequest(requestDER(t, "localhost", strings.Repeat("b", 64), testX224), testNonce); !errors.Is(err, ErrAuthentication) {
		t.Fatal(err)
	}
	for _, nonce := range []string{"", strings.Repeat("g", 64), strings.Repeat("a", 62)} {
		if _, err := decodeRequest(requestDER(t, "localhost", nonce, testX224), nonce); !errors.Is(err, ErrAuthentication) {
			t.Fatal("invalid expected nonce accepted")
		}
	}
}

func TestRDCleanPathRejectsAmbiguousDER(t *testing.T) {
	valid := requestDER(t, "localhost", testNonce, testX224)
	wrap := func(body []byte) []byte {
		out, err := asn1.Marshal(asn1.RawValue{Class: 0, Tag: 16, IsCompound: true, Bytes: body})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	var outer asn1.RawValue
	_, _ = asn1.Unmarshal(valid, &outer)
	body := outer.Bytes
	fields := make([][]byte, 0, 4)
	for len(body) > 0 {
		var v asn1.RawValue
		next, err := asn1.Unmarshal(body, &v)
		if err != nil {
			t.Fatal(err)
		}
		fields = append(fields, v.FullBytes)
		body = next
	}
	join := func(parts ...[]byte) []byte { return wrap(bytes.Join(parts, nil)) }
	cases := map[string][]byte{
		"unknown":            join(fields[0], fields[1], fields[2], fields[3], []byte{0xaa, 2, 5, 0}),
		"server_auth":        join(fields[0], fields[1], fields[2], []byte{0xa4, 2, 12, 0}, fields[3]),
		"pcb":                join(fields[0], fields[1], fields[2], []byte{0xa5, 2, 12, 0}, fields[3]),
		"duplicate":          join(fields[0], fields[1], fields[1], fields[2], fields[3]),
		"reordered":          join(fields[0], fields[2], fields[1], fields[3]),
		"missing_nonce":      join(fields[0], fields[1], fields[3]),
		"wrong_version":      join([]byte{0xa0, 4, 2, 2, 0x0d, 0x3f}, fields[1], fields[2], fields[3]),
		"nonminimal_integer": join([]byte{0xa0, 5, 2, 3, 0, 0x0d, 0x3e}, fields[1], fields[2], fields[3]),
		"trailing_outer":     append(append([]byte(nil), valid...), 0),
		"nonminimal_length":  append([]byte{0x30, 0x81, valid[1]}, valid[2:]...),
		"indefinite":         append([]byte{0x30, 0x80}, valid[2:]...),
		"truncated":          valid[:len(valid)-1],
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeRequest(data, testNonce); err == nil {
				t.Fatal("ambiguous DER accepted")
			}
		})
	}
	for i := 0; i < len(valid); i++ {
		if _, err := decodeRequest(valid[:i], testNonce); err == nil {
			t.Fatalf("prefix %d accepted", i)
		}
	}
}

type fragmentReader struct{ r io.Reader }

func (r fragmentReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.r.Read(p)
}

func TestReadDERFragmentsAndPreservesNextPDU(t *testing.T) {
	data := requestDER(t, "localhost", testNonce, testX224)
	r := bytes.NewReader(append(append([]byte(nil), data...), 1, 2, 3))
	got, err := readDER(fragmentReader{r})
	if err != nil || !bytes.Equal(got, data) || r.Len() != 3 {
		t.Fatalf("fragment read: %v remaining=%d", err, r.Len())
	}
	for _, data := range [][]byte{{0x30, 0x80}, {0x30, 0x83, 1, 0, 0}, {0x30, 0x82, 0xff, 0xff}, {0x30, 0x81, 0x7f}, {0x30, 0x82, 0, 0x80}, {0x31, 0}} {
		if _, err := readDER(bytes.NewReader(data)); err == nil {
			t.Fatalf("bad length accepted: %x", data)
		}
	}
}

func confirmation(protocol uint32) []byte {
	p := []byte{3, 0, 0, 19, 14, 0xd0, 0, 0, 0, 0, 0, 2, 0, 8, 0, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(p[15:], protocol)
	return p
}

func TestX224RequiresNLAAndCompletePackets(t *testing.T) {
	for _, protocol := range []uint32{2, 8} {
		got, err := readX224Response(fragmentReader{bytes.NewReader(confirmation(protocol))}, 10)
		if err != nil || !bytes.Equal(got, confirmation(protocol)) {
			t.Fatal(err)
		}
	}
	for _, protocol := range []uint32{0, 1, 4, 3, 16} {
		if _, err := readX224Response(bytes.NewReader(confirmation(protocol)), 15); !errors.Is(err, ErrNLARequired) {
			t.Fatalf("accepted protocol %d: %v", protocol, err)
		}
	}
	if _, err := readX224Response(bytes.NewReader(confirmation(8)), 3); err == nil {
		t.Fatal("unoffered protocol accepted")
	}
	for i := 0; i < 19; i++ {
		if _, err := readX224Response(bytes.NewReader(confirmation(2)[:i]), 3); err == nil {
			t.Fatalf("accepted partial packet %d", i)
		}
	}
	for _, index := range []int{0, 1, 2, 3, 4, 5, 10, 11, 13, 14} {
		p := confirmation(2)
		p[index] ^= 0xff
		if _, err := readX224Response(bytes.NewReader(p), 3); err == nil {
			t.Fatalf("invalid byte %d accepted", index)
		}
	}
	sslOnly := append([]byte(nil), testX224...)
	sslOnly[15] = 1
	if _, err := validateX224Request(sslOnly); !errors.Is(err, ErrNLARequired) {
		t.Fatal(err)
	}
	cookie := append(append([]byte(nil), testX224[:11]...), []byte("Cookie: mstshash=alice\r\n")...)
	cookie = append(cookie, testX224[11:]...)
	cookie[4] = byte(len(cookie) - 5)
	binary.BigEndian.PutUint16(cookie[2:], uint16(len(cookie)))
	if _, err := validateX224Request(cookie); err != nil {
		t.Fatal(err)
	}
	correlation := append(append([]byte(nil), testX224...), make([]byte, 36)...)
	correlation[12] = 8
	correlation[19] = 6
	correlation[21] = 36
	correlation[4] = byte(len(correlation) - 5)
	binary.BigEndian.PutUint16(correlation[2:], uint16(len(correlation)))
	if _, err := validateX224Request(correlation); err != nil {
		t.Fatal(err)
	}
	correlation[len(correlation)-1] = 1
	if _, err := validateX224Request(correlation); err == nil {
		t.Fatal("nonzero reserved bytes accepted")
	}
}

func FuzzRDCleanPath(f *testing.F) {
	f.Add([]byte{0x30, 0})
	f.Add(unhexF("306ea00402020d3ea20b0c096c6f63616c686f7374a3420c40" + strings.Repeat("61", 64) + "a6150413030000130ee000000000000100080003000000"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > maxHandshakeBytes+10 {
			t.Skip()
		}
		_, _ = decodeRequest(b, testNonce)
		_, _ = readDER(bytes.NewReader(b))
	})
}
func unhexF(s string) []byte { b, _ := hex.DecodeString(s); return b }
