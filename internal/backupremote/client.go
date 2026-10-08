package backupremote

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/netpolicy"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Object struct {
	Key      string    `json:"key"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}
type Client struct {
	storage   Storage
	http      *http.Client
	s3        *minio.Client
	transport *http.Transport
}

// Administrators may connect a private NAS/MinIO. Loopback, metadata and
// non-unicast addresses remain unavailable; DNS is checked at every dial.
func allowedIP(ip netip.Addr) bool {
	switch netpolicy.Classify(ip) {
	case netpolicy.Public, netpolicy.Private:
		return true
	default:
		return false
	}
}
func secureDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrInvalid
	}
	lookup, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(lookup, "ip", host)
	if err != nil || len(ips) == 0 || len(ips) > 16 {
		return nil, ErrUnavailable
	}
	for _, ip := range ips {
		if !allowedIP(ip) {
			return nil, ErrUnavailable
		}
	}
	dialer := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	for _, ip := range ips {
		conn, e := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return idleConn{conn}, nil
		}
		err = e
	}
	return nil, err
}

// Bound stalled object bodies without imposing a short total transfer limit.
type idleConn struct{ net.Conn }

func (c idleConn) Read(p []byte) (int, error) {
	_ = c.SetReadDeadline(time.Now().Add(time.Minute))
	return c.Conn.Read(p)
}
func (c idleConn) Write(p []byte) (int, error) {
	_ = c.SetWriteDeadline(time.Now().Add(time.Minute))
	return c.Conn.Write(p)
}

type boundedTransport struct {
	base     http.RoundTripper
	endpoint *url.URL
	bucket   string
	awsHost  string
}

func (t boundedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// SDK region retries and server redirects must not forward credentials to a
	// host other than the configured endpoint (or its configured bucket).
	host := r.URL.Hostname()
	allowedHost := host == t.endpoint.Hostname() || host == t.bucket+"."+t.endpoint.Hostname() || t.awsHost != "" && (host == t.awsHost || host == t.bucket+"."+t.awsHost)
	if r.URL.Scheme != t.endpoint.Scheme || effectivePort(r.URL) != effectivePort(t.endpoint) || !allowedHost {
		return nil, ErrUnavailable
	}
	res, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, ErrUnavailable
	}
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		res.Body.Close()
		return nil, ErrUnavailable
	}
	// Protocol XML and error documents are always bounded. Only object GETs
	// may carry the package-sized stream, bounded again by Download.
	limit := int64(4 << 20)
	if r.Method == "GET" && r.URL.RawQuery == "" && res.StatusCode == 200 {
		limit = backup.MaxEncryptedBytes
	}
	res.Body = &limitedBody{Reader: io.LimitReader(res.Body, limit+1), Closer: res.Body}
	return res, nil
}

type limitedBody struct {
	io.Reader
	io.Closer
}

func NewClient(storage Storage) (*Client, error) { return newClient(storage, nil) }

// The injectable transport is package-private and used only by loopback tests.
func newClient(storage Storage, injected http.RoundTripper) (*Client, error) {
	if storage.Validate() != nil {
		return nil, ErrInvalid
	}
	u, _ := url.Parse(storage.Endpoint)
	transport := &http.Transport{Proxy: nil, DialContext: secureDial, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DisableCompression: true, MaxConnsPerHost: 3, MaxIdleConns: 3, MaxIdleConnsPerHost: 3, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, MaxResponseHeaderBytes: 64 << 10}
	var base http.RoundTripper = transport
	if injected != nil {
		base = injected
	}
	guard := boundedTransport{base: base, endpoint: u, bucket: storage.Bucket}
	// The SDK canonicalizes standard AWS endpoints even with an explicit
	// region. Allow only that deterministic alias, never a response-supplied
	// redirect host or a wildcard AWS domain.
	if storage.Kind == "s3" {
		suffix := ".amazonaws.com"
		if strings.HasPrefix(storage.Region, "cn-") {
			suffix += ".cn"
		}
		regional := "s3." + storage.Region + suffix
		host := u.Hostname()
		if host == "s3.amazonaws.com" || host == regional || host == "s3-"+storage.Region+suffix || host == "s3.dualstack."+storage.Region+suffix {
			guard.awsHost = regional
		}
	}
	c := &Client{storage: storage, transport: transport, http: &http.Client{Transport: guard, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrUnavailable }}}
	if storage.Kind == "s3" {
		lookup := minio.BucketLookupDNS
		if storage.PathStyle {
			lookup = minio.BucketLookupPath
		}
		client, err := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(storage.AccessKey, storage.Secret, ""), Secure: u.Scheme == "https", Region: storage.Region, BucketLookup: lookup, Transport: guard, MaxRetries: 2})
		if err != nil {
			return nil, ErrInvalid
		}
		c.s3 = client
		client.SetS3EnableDualstack(false)
	}
	return c, nil
}
func (c *Client) Close()                 { c.transport.CloseIdleConnections() }
func (c *Client) key(name string) string { return path.Join(c.storage.Prefix, name) }

func (c *Client) Upload(ctx context.Context, name string, file *os.File, size int64) error {
	if !ValidObject(name) || size <= 0 || size > backup.MaxEncryptedBytes {
		return ErrInvalid
	}
	return c.put(ctx, name, file, size)
}
func (c *Client) put(ctx context.Context, name string, reader io.Reader, size int64) error {
	if c.s3 != nil {
		_, err := c.s3.PutObject(ctx, c.storage.Bucket, c.key(name), reader, size, minio.PutObjectOptions{ContentType: "application/octet-stream", PartSize: 16 << 20, NumThreads: 1})
		if err != nil {
			return ErrUnavailable
		}
	} else if err := c.davPut(ctx, name, reader, size); err != nil {
		return err
	}
	info, err := c.stat(ctx, name)
	if err != nil || info.Size != size {
		return ErrUnavailable
	}
	return nil
}
func (c *Client) stat(ctx context.Context, name string) (Object, error) {
	if c.s3 != nil {
		v, err := c.s3.StatObject(ctx, c.storage.Bucket, c.key(name), minio.StatObjectOptions{})
		if err != nil {
			return Object{}, ErrUnavailable
		}
		return Object{Key: name, Size: v.Size, Modified: v.LastModified}, nil
	}
	res, err := c.davRequest(ctx, "HEAD", name, nil, 0, nil)
	if err != nil {
		return Object{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || res.ContentLength < 0 {
		return Object{}, ErrUnavailable
	}
	modified, _ := http.ParseTime(res.Header.Get("Last-Modified"))
	return Object{Key: name, Size: res.ContentLength, Modified: modified}, nil
}
func (c *Client) get(ctx context.Context, name string) (io.ReadCloser, error) {
	if c.s3 != nil {
		obj, err := c.s3.GetObject(ctx, c.storage.Bucket, c.key(name), minio.GetObjectOptions{})
		if err != nil {
			return nil, ErrUnavailable
		}
		return obj, nil
	}
	res, err := c.davRequest(ctx, "GET", name, nil, 0, nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 || res.Header.Get("Content-Encoding") != "" && res.Header.Get("Content-Encoding") != "identity" {
		res.Body.Close()
		return nil, ErrUnavailable
	}
	return res.Body, nil
}
func (c *Client) Download(ctx context.Context, name, target string) (int64, error) {
	if !ValidObject(name) {
		return 0, ErrInvalid
	}
	info, err := c.stat(ctx, name)
	if err != nil || info.Size <= 0 || info.Size > backup.MaxEncryptedBytes {
		return 0, ErrUnavailable
	}
	if err := backup.RequireSpace(filepath.Dir(target), info.Size*3); err != nil {
		return 0, err
	}
	r, err := c.get(ctx, name)
	if err != nil {
		return 0, err
	}
	n, err := backup.CopyFile(target, r, info.Size)
	err = errors.Join(err, r.Close())
	if err != nil || n != info.Size {
		_ = os.Remove(target)
		return 0, ErrUnavailable
	}
	return n, nil
}
func (c *Client) Delete(ctx context.Context, name string) error {
	if !ValidObject(name) {
		return ErrInvalid
	}
	return c.remove(ctx, name)
}
func (c *Client) remove(ctx context.Context, name string) error {
	if c.s3 != nil {
		if err := c.s3.RemoveObject(ctx, c.storage.Bucket, c.key(name), minio.RemoveObjectOptions{}); err != nil {
			return ErrUnavailable
		}
		return nil
	}
	res, err := c.davRequest(ctx, "DELETE", name, nil, 0, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 204 && res.StatusCode != 404 {
		return ErrUnavailable
	}
	return nil
}
func (c *Client) List(ctx context.Context) ([]Object, error) {
	if c.s3 == nil {
		return c.davList(ctx)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	prefix := c.storage.Prefix
	if prefix != "" {
		prefix += "/"
	}
	out := []Object{}
	count := 0
	for v := range c.s3.ListObjects(ctx, c.storage.Bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: false, MaxKeys: MaxObjects}) {
		count++
		if count > MaxObjects {
			return nil, ErrLimit
		}
		if v.Err != nil {
			return nil, ErrUnavailable
		}
		name := strings.TrimPrefix(v.Key, prefix)
		if ValidObject(name) && v.Size > 0 && v.Size <= backup.MaxEncryptedBytes {
			out = append(out, Object{Key: name, Size: v.Size, Modified: v.LastModified})
		}
	}
	return out, nil
}
func (c *Client) Test(ctx context.Context) error {
	name := ".kpanel-test-" + backup.NewID()
	content := []byte("KPanel remote backup connection test\n")
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = c.remove(cleanup, name)
	}()
	if err := c.put(ctx, name, bytes.NewReader(content), int64(len(content))); err != nil {
		return err
	}
	r, err := c.get(ctx, name)
	if err != nil {
		return err
	}
	got, err := io.ReadAll(io.LimitReader(r, int64(len(content)+1)))
	err = errors.Join(err, r.Close())
	if err != nil || !bytes.Equal(got, content) {
		return ErrUnavailable
	}
	if _, err := c.List(ctx); err != nil {
		return err
	}
	return c.remove(ctx, name)
}

func validPort(u *url.URL) bool {
	p := u.Port()
	if p == "" {
		return true
	}
	n, e := strconv.Atoi(p)
	return e == nil && n > 0 && n <= 65535
}

func effectivePort(u *url.URL) string {
	if u.Port() != "" {
		return u.Port()
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}
