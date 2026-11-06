package redact

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// These representative records were emitted by the pinned, network-isolated
// Nginx image during this candidate's validation (loopback requests only).
const realNginxRecords = "2026/11/06 02:55:56 [notice] 7#7: using the \"epoll\" event method\n" +
	"127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET / HTTP/1.1\" 200 896 \"-\" \"Wget\" \"-\"\n" +
	"2026/11/06 02:55:56 [error] 11#11: *2 open() \"/usr/share/nginx/html/fixture-missing\" failed (2: No such file or directory), client: 127.0.0.1, server: localhost, request: \"GET /fixture-missing HTTP/1.1\", host: \"127.0.0.1\"\n"

type tinyChunkReader struct {
	source io.Reader
	max    int
}

type repeatedRecordReader struct {
	record            []byte
	remaining, offset int
}

func (r *repeatedRecordReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if len(p) > r.remaining {
		p = p[:r.remaining]
	}
	written := 0
	for written < len(p) {
		n := copy(p[written:], r.record[r.offset:])
		written += n
		r.offset = (r.offset + n) % len(r.record)
	}
	r.remaining -= written
	return written, nil
}

func BenchmarkConservativeLogTail128MiB(b *testing.B) {
	size := (128 << 20) / len(realNginxRecords) * len(realNginxRecords)
	b.SetBytes(int64(size))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reader := &repeatedRecordReader{record: []byte(realNginxRecords), remaining: size}
		if _, _, err := ConservativeLogTail(context.Background(), reader, 128<<20, 16<<10); err != nil {
			b.Fatal(err)
		}
	}
}

func (r tinyChunkReader) Read(p []byte) (int, error) {
	if len(p) > r.max {
		p = p[:r.max]
	}
	return r.source.Read(p)
}

func TestConservativeLogTailLargeRealRecords(t *testing.T) {
	corpus := strings.Repeat(realNginxRecords, (20<<20)/len(realNginxRecords)+1)
	result, truncated, err := ConservativeLogTail(context.Background(), strings.NewReader(corpus), 32<<20, 16<<10)
	if err != nil || !truncated || !strings.Contains(result, "fixture-missing") || len(result) > 16<<10 {
		t.Fatalf("large log truncated=%v len=%d err=%v", truncated, len(result), err)
	}
}

func TestConservativeLogTailMasksSensitiveAccessRecords(t *testing.T) {
	first := "127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /normal HTTP/1.1\" 200 10 \"-\" \"Wget\" \"-\"\n"
	for _, query := range []string{"?token=OPAQUE_BODY", "?sig.foo=OPAQUE_BODY", "?password=OPAQUE_BODY", "?%74oken=OPAQUE_BODY", "?user=alice@example.invalid"} {
		secret := "127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /search" + query + " HTTP/1.1\" 200 10 \"-\" \"Wget\" \"-\"\n"
		result, _, err := ConservativeLogTail(context.Background(), tinyChunkReader{strings.NewReader(first + secret + first), 7}, 1<<20, 1024)
		if err != nil || strings.Contains(result, "OPAQUE_BODY") || !strings.Contains(result, "[REDACTED LOG RECORD]") || !strings.HasSuffix(result, first) {
			t.Fatalf("sensitive access result=%q err=%v", result, err)
		}
	}
}

func TestConservativeLogTailRejectsCrossRecordSecrets(t *testing.T) {
	for _, secret := range []string{
		"apiVersion: v1\nkind: Secret\ndata:\n  opaque: SYNTHETIC_PRIVATE\n",
		"data:\n  opaque: SYNTHETIC_PRIVATE\nkind: Secret\n",
		"data:\n  opaque: OPAQUE_BODY\n",
		"- \"pass\\u0077ord\": SYNTHETIC_PRIVATE\n",
		"Authorization: Bearer SYNTHETIC_PRIVATE\n",
		`{"name":"opaque","value":"SYNTHETIC_PRIVATE"}` + "\n",
		"docker login -p SYNTHETIC_PRIVATE\n",
		"DOCKER LOGIN -pSYNTHETIC_PRIVATE\n",
		"mariadb -pOPAQUE_BODY\n",
		"mariadb-dump -p OPAQUE_BODY\n",
		"ſecret: OPAQUE_BODY\n",
		"paſſword: OPAQUE_BODY\n",
		"ſecret:\nOPAQUE_BODY\n",
		"ſk-OPAQUEBODY\n",
		"https://example.invalid/?sig.foo=OPAQUE_BODY\n",
		"{\"x\":\"\nOPAQUE_BODY\n\"}\n",
		"? benign\nopaque: OPAQUE_BODY\n",
		"127.0.0.1 - password=' [06/Nov/2026:02:55:56 +0000] \"GET / HTTP/1.1\" 200 10 \"-\" \"Wget\" \"-\"\nOPAQUE_BODY\n'\n",
		"127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /?token=$(open HTTP/1.1\" 200 10 \"-\" \"Wget\" \"-\"\nOPAQUE_BODY\n",
		"127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /?token=${open HTTP/1.1\" 200 10 \"-\" \"Wget\" \"-\"\nOPAQUE_BODY\n",
		"127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /?token=`open HTTP/1.1\" 200 10 \"-\" \"Wget\" \"-\"\nOPAQUE_BODY\n",
		"127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /?token=[1 HTTP/1.1\" 200 10 \"-\" \"Wget\" \"-\"\nOPAQUE_BODY\n",
		"127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /{? hidden}?token=opaque HTTP/1.1\" 200 10 \"-\" \"Wget\" \"-\"\nOPAQUE_BODY\n",
	} {
		for _, data := range []string{secret + realNginxRecords, realNginxRecords + secret} {
			result, _, err := ConservativeLogTail(context.Background(), tinyChunkReader{strings.NewReader(data), 3}, 1<<20, 64)
			if result != "" || !errors.Is(err, ErrStreamAmbiguous) {
				t.Fatalf("unsafe stream result=%q err=%v", result, err)
			}
		}
	}
}

func TestConservativeLogFastPathMatchesText(t *testing.T) {
	parts := []string{"alpha", "beta", "127.0.0.1", "[notice]", "[10]", "{", "}", "[", "]", `"`, "'", ":", "=", "?sig.foo=opaque", "ſecret", "Key", "-p", "docker", "login", "\\", "%", "@", " ", "\t"}
	rng := rand.New(rand.NewSource(73))
	for i := 0; i < 5000; i++ {
		var input strings.Builder
		for j := 0; j < 1+rng.Intn(15); j++ {
			input.WriteString(parts[rng.Intn(len(parts))])
		}
		input.WriteByte('\n')
		view, _, accepted := independentPlainRecord([]byte(input.String()))
		if accepted && !streamMayContainStructure(string(view)) && Text(string(view)) != string(view) {
			t.Fatalf("fast path disagreed with full projection on %q", input.String())
		}
	}
}

func TestConservativeLogMultiRecordMatchesText(t *testing.T) {
	parts := []string{"alpha", "beta", "?", "? benign", "OPAQUE_BODY", "[notice]", "[10]", "{", "}", `"`, "'", ":", "=", "?sig.foo=opaque", "ſecret", "kind", "name", "value", "-p", "docker", "login", "\\", "%", "@", " ", "\t"}
	rng := rand.New(rand.NewSource(74))
	for i := 0; i < 5000; i++ {
		var input strings.Builder
		for line := 0; line < 3; line++ {
			for j := 0; j < 1+rng.Intn(6); j++ {
				input.WriteString(parts[rng.Intn(len(parts))])
			}
			input.WriteByte('\n')
		}
		value := input.String()
		result, _, err := ConservativeLogTail(context.Background(), strings.NewReader(value), 1024, 1024)
		if err == nil && (result != value || Text(value) != value) {
			t.Fatalf("accepted projection disagreement on %q", value)
		}
	}
}

func TestConservativeLogRecognizedSensitiveClasses(t *testing.T) {
	for _, sample := range []string{
		"mysql -popaque\n", "mysqldump -p opaque\n", "mariadb -popaque\n", "mariadb-dump -p opaque\n",
		"curl -u admin:opaque https://example.invalid\n", "docker login -popaque example.invalid\n",
		"Authorization: Basic dTpw\n", "Basic dTpw\n", "Bearer abcdef\n", "sk-abcdef\n",
		"https://u:opaque@example.invalid/\n", "https://example.invalid/?sig.foo=opaque\n",
		"password: opaque\n", "access_key: opaque\n", "tls.key: opaque\n", "密码: opaque\n",
		`{"name":"opaque","value":"opaque"}` + "\n", "ſecret: opaque\n", "? benign\nopaque: value\n",
	} {
		if Text(sample) == sample {
			t.Fatalf("fixture was not sensitive to ordinary projection: %q", sample)
		}
		result, _, err := ConservativeLogTail(context.Background(), tinyChunkReader{strings.NewReader(sample), 3}, 4096, 1024)
		if err == nil || result != "" {
			t.Fatalf("recognized sensitive input accepted: %q result=%q", sample, result)
		}
	}
}

func TestConservativeLogTailRejectsCrossBatchContext(t *testing.T) {
	pad := strings.Repeat(strings.Repeat("x", 63)+"\n", 1023)
	for _, input := range []string{
		pad + "kind: Secret\n" + strings.Repeat("x", 60) + "\n  opaque: OPAQUE_BODY\n",
		pad + "data:\n  opaque: OPAQUE_BODY\n" + strings.Repeat("x", 60) + "\nkind: Secret\n",
		pad + "password: |\n" + strings.Repeat("x", 60) + "\n  OPAQUE_BODY\n",
		pad + "\"pass\\u0077ord\":\n" + strings.Repeat("x", 60) + "\n  OPAQUE_BODY\n",
		pad + "{\"x\":\"\n" + strings.Repeat("x", 60) + "\nOPAQUE_BODY\n\"}\n",
		pad + "? benign\n" + strings.Repeat("x", 60) + "\nopaque: OPAQUE_BODY\n",
		pad + "127.0.0.1 - password=' [06/Nov/2026:02:55:56 +0000] \"GET / HTTP/1.1\" 200 10 \"-\" \"Wget\" \"-\"\nOPAQUE_BODY\n'\n",
		pad + "127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /?token=$(open HTTP/1.1\" 200 10 \"-\" \"Wget\" \"-\"\nOPAQUE_BODY\n",
		pad + "127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /?token=[1 HTTP/1.1\" 200 10 \"-\" \"Wget\" \"-\"\nOPAQUE_BODY\n",
	} {
		result, _, err := ConservativeLogTail(context.Background(), tinyChunkReader{strings.NewReader(input), 11}, 1<<20, 64)
		if result != "" || err == nil {
			t.Fatalf("cross-batch context accepted result=%q err=%v", result, err)
		}
	}
}

func TestConservativeLogTailMasksPrivateKeysAcrossBatches(t *testing.T) {
	pad := strings.Repeat(strings.Repeat("x", 63)+"\n", 1023)
	for _, key := range []string{
		"-----BEGIN PRIVATE KEY-----\nOPAQUEBODY\n-----END PRIVATE KEY-----\n",
		"OPAQUE_BODY\n-----END PRIVATE KEY-----\n",
	} {
		result, _, err := ConservativeLogTail(context.Background(), tinyChunkReader{strings.NewReader(pad + key + realNginxRecords), 11}, 1<<20, 4096)
		if err != nil || strings.Contains(result, "OPAQUE_BODY") || strings.Contains(result, "OPAQUEBODY") || !strings.Contains(result, "[REDACTED PRIVATE KEY]") || !strings.Contains(result, "fixture-missing") {
			t.Fatalf("private key projection result=%q err=%v", result, err)
		}
	}
	for _, key := range []string{
		"-----BEGIN PRIVATE KEY-----\n{\"line\":\"-----BEGIN PRIVATE KEY-----\"}\n-----END PRIVATE KEY-----\n{\"line\":\"OPAQUE_BODY\"}\n",
		"-----BEGIN PRIVATE KEY-----\n{\"other\":\"-----END PRIVATE KEY-----\"}\nOPAQUE_BODY\n-----END PRIVATE KEY-----\n",
		"-----BEGIN PRIVATE KEY-----\n\\u002d\\u002dOPAQUE_BODY\n-----END PRIVATE KEY-----\n",
		"-----BEGIN PRIVATE KEY-----\nkind: Secret\n-----END PRIVATE KEY-----\n{\"line\":\"OPAQUE_BODY\"}\n",
		"{\"x\":\"\n-----END PRIVATE KEY-----\nOPAQUE_BODY\n",
		"-----BEGIN RSA PRIVATE KEY-----\nQQ==\n-----END EC PRIVATE KEY-----\nOPAQUE_BODY\n",
	} {
		result, _, err := ConservativeLogTail(context.Background(), tinyChunkReader{strings.NewReader(pad + key + realNginxRecords), 11}, 1<<20, 4096)
		if result != "" || !errors.Is(err, ErrStreamAmbiguous) {
			t.Fatalf("nested private context accepted result=%q err=%v", result, err)
		}
	}
}

type cancelAfterRead struct {
	cancel context.CancelFunc
	called bool
}

func (r *cancelAfterRead) Read(p []byte) (int, error) {
	if r.called {
		return 0, io.EOF
	}
	r.called = true
	n := copy(p, "ordinary line\n")
	r.cancel()
	return n, nil
}

func TestConservativeLogTailCancellationAndBudgets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	result, _, err := ConservativeLogTail(ctx, &cancelAfterRead{cancel: cancel}, 1024, 64)
	if result != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation result=%q err=%v", result, err)
	}
	result, _, err = ConservativeLogTail(context.Background(), strings.NewReader(strings.Repeat("x", (64<<10)+1)), 1<<20, 64)
	if result != "" || !errors.Is(err, ErrStreamLimit) {
		t.Fatalf("long line result=%q err=%v", result, err)
	}
	result, _, err = ConservativeLogTail(context.Background(), strings.NewReader(strings.Repeat("normal\n", 100)), 20, 64)
	if result != "" || !errors.Is(err, ErrStreamLimit) {
		t.Fatalf("input budget result=%q err=%v", result, err)
	}
}

type byteCountingReader struct {
	source io.Reader
	bytes  int
}

type stalledReader struct{ entered, release chan struct{} }

func (r *stalledReader) Read(p []byte) (int, error) {
	close(r.entered)
	<-r.release
	return copy(p, "ordinary line\n"), nil
}

func TestConservativeLogTailCanceledDuringBlockedRead(t *testing.T) {
	reader := &stalledReader{entered: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type outcome struct {
		result string
		err    error
	}
	done := make(chan outcome, 1)
	go func() { value, _, err := ConservativeLogTail(ctx, reader, 1024, 64); done <- outcome{value, err} }()
	select {
	case <-reader.entered:
	case <-time.After(time.Second):
		t.Fatal("reader was not entered")
	}
	cancel()
	select {
	case result := <-done:
		close(reader.release)
		if result.result != "" || !errors.Is(result.err, context.Canceled) {
			t.Fatalf("early cancellation: %#v", result)
		}
	case <-time.After(50 * time.Millisecond):
		close(reader.release)
		select {
		case result := <-done:
			if result.result != "" || !errors.Is(result.err, context.Canceled) {
				t.Fatalf("cancellation after unblock: %#v", result)
			}
		case <-time.After(time.Second):
			t.Fatal("did not finish after unblock")
		}
	}
}

func (r *byteCountingReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	r.bytes += n
	return n, err
}

func TestConservativeLogTailPreservesCompleteBoundaryAndReadLimit(t *testing.T) {
	last := strings.Repeat("x", 1023) + "\n"
	result, truncated, err := ConservativeLogTail(context.Background(), strings.NewReader("ok\n"+last), 2048, 1024)
	if err != nil || !truncated || result != last {
		t.Fatalf("boundary result length=%d truncated=%v err=%v", len(result), truncated, err)
	}
	reader := &byteCountingReader{source: strings.NewReader(strings.Repeat("x", 100000))}
	result, _, err = ConservativeLogTail(context.Background(), reader, 1, 64)
	if result != "" || !errors.Is(err, ErrStreamLimit) || reader.bytes > 2 {
		t.Fatalf("read limit result=%q bytes=%d err=%v", result, reader.bytes, err)
	}
}

func TestConservativeLogTailMatchesReferenceBoundaries(t *testing.T) {
	rng := rand.New(rand.NewSource(75))
	for i := 0; i < 500; i++ {
		var input strings.Builder
		for j := 0; j < 10+rng.Intn(200); j++ {
			input.WriteString(strings.Repeat("x", rng.Intn(40)))
			input.WriteString("行\n")
		}
		value := input.String()
		limit := 1 + rng.Intn(512)
		want := value
		truncated := len(value) > limit
		if truncated {
			cut := len(value) - limit
			want = value[cut:]
			if value[cut-1] != '\n' {
				if newline := strings.IndexByte(want, '\n'); newline >= 0 {
					want = want[newline+1:]
				}
				for len(want) > 0 && !utf8.ValidString(want) {
					want = want[1:]
				}
			}
		}
		got, actualTruncated, err := ConservativeLogTail(context.Background(), strings.NewReader(value), 1<<20, limit)
		if err != nil || got != want || actualTruncated != truncated {
			t.Fatalf("iteration=%d limit=%d got=%q want=%q truncated=%v err=%v", i, limit, got, want, actualTruncated, err)
		}
	}
}
