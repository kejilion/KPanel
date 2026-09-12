// Package backup implements bounded, authenticated, portable backup containers.
// Host payloads are opaque here; only their owning adapter may interpret them.
package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/scrypt"
)

const (
	MaxBytes          int64 = 50 << 30
	MaxPanelBytes     int64 = 32 << 20
	MaxEncryptedBytes int64 = MaxBytes + (32 << 20)
	chunkSize               = 64 << 10
	FormatVersion           = 1
)

var ErrInvalid = errors.New("backup is invalid, damaged, incompatible, or the password is incorrect")
var Modules = []string{"panel", "apps", "web", "docker"}
var magic = []byte("KPANELBK\x01")

type Part struct {
	Module string `json:"module"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	Format    int       `json:"format"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	Parts     []Part    `json:"parts"`
}
type Source struct{ Module, Path string }

func Selection(selected []string) ([]string, error) {
	if len(selected) == 0 || len(selected) > len(Modules) {
		return nil, ErrInvalid
	}
	result := []string{}
	for _, module := range Modules {
		if slices.Contains(selected, module) {
			result = append(result, module)
		}
	}
	if len(result) != len(selected) {
		return nil, ErrInvalid
	}
	return result, nil
}

func ValidatePassword(password string) error {
	if len(password) < 10 || len(password) > 256 {
		return errors.New("backup password must contain 10 to 256 bytes")
	}
	return nil
}

func Write(w io.Writer, password, version string, sources []Source) (Manifest, error) {
	return WriteContext(context.Background(), w, password, version, sources)
}
func WriteContext(ctx context.Context, w io.Writer, password, version string, sources []Source) (Manifest, error) {
	w = contextWriter{ctx, w}
	manifest := Manifest{Format: FormatVersion, Version: version, CreatedAt: time.Now().UTC()}
	selected := []string{}
	var total int64
	for _, source := range sources {
		selected = append(selected, source.Module)
		f, err := OpenRegular(source.Path, MaxBytes)
		if err != nil {
			return manifest, err
		}
		hash := sha256.New()
		n, err := io.Copy(hash, io.LimitReader(contextReader{ctx, f}, MaxBytes+1))
		f.Close()
		total += n
		if err != nil || n > MaxBytes || total > MaxBytes || source.Module == "panel" && n > MaxPanelBytes {
			return manifest, ErrInvalid
		}
		manifest.Parts = append(manifest.Parts, Part{source.Module, n, hex.EncodeToString(hash.Sum(nil))})
	}
	if _, err := Selection(selected); err != nil {
		return manifest, err
	}
	pipeR, pipeW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		tw := tar.NewWriter(pipeW)
		body, err := json.Marshal(manifest)
		if err == nil {
			err = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(body)), Typeflag: tar.TypeReg})
		}
		if err == nil {
			_, err = tw.Write(body)
		}
		for i, source := range sources {
			if err != nil {
				break
			}
			part := manifest.Parts[i]
			err = tw.WriteHeader(&tar.Header{Name: source.Module + ".payload", Mode: 0600, Size: part.Size, Typeflag: tar.TypeReg})
			if err != nil {
				break
			}
			var f *os.File
			f, err = OpenRegular(source.Path, MaxBytes)
			if err != nil {
				break
			}
			hash := sha256.New()
			_, err = io.CopyN(io.MultiWriter(tw, hash), f, part.Size)
			var extra [1]byte
			n, tailErr := f.Read(extra[:])
			f.Close()
			if err == nil && (n != 0 || tailErr != io.EOF || hex.EncodeToString(hash.Sum(nil)) != part.SHA256) {
				err = ErrInvalid
			}
		}
		if err == nil {
			err = tw.Close()
		}
		pipeW.CloseWithError(err)
		done <- err
	}()
	err := encrypt(w, pipeR, password)
	pipeR.CloseWithError(err)
	producerErr := <-done
	if err == nil {
		err = producerErr
	}
	return manifest, err
}

// Read verifies every authentication tag including an explicit final frame
// before returning. No host action is performed while reading an archive.
func Read(r io.Reader, password, directory string) (Manifest, error) {
	return ReadContext(context.Background(), r, password, directory)
}
func ReadContext(ctx context.Context, r io.Reader, password, directory string) (manifest Manifest, resultErr error) {
	r = contextReader{ctx, r}
	created := []string{}
	defer func() {
		if resultErr != nil {
			for _, path := range created {
				_ = os.Remove(path)
			}
		}
	}()
	plain, err := os.OpenFile(filepath.Join(directory, "container.tmp"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return manifest, err
	}
	defer func() { plain.Close(); os.Remove(plain.Name()) }()
	if err := decrypt(plain, r, password); err != nil {
		return manifest, err
	}
	if _, err := plain.Seek(0, io.SeekStart); err != nil {
		return manifest, err
	}
	tr := tar.NewReader(contextReader{ctx, plain})
	h, err := tr.Next()
	if err != nil || h.Name != "manifest.json" || h.Typeflag != tar.TypeReg || h.Size > 16<<10 {
		return manifest, ErrInvalid
	}
	body, err := io.ReadAll(tr)
	if err != nil || Decode(body, &manifest) != nil || manifest.Format != FormatVersion || len(manifest.Version) > 64 || manifest.CreatedAt.IsZero() {
		return manifest, ErrInvalid
	}
	selected := []string{}
	var total int64
	for _, part := range manifest.Parts {
		selected = append(selected, part.Module)
		total += part.Size
		if part.Size < 0 || total > MaxBytes || part.Size > MaxBytes || part.Module == "panel" && part.Size > MaxPanelBytes || len(part.SHA256) != 64 {
			return manifest, ErrInvalid
		}
	}
	if _, err := Selection(selected); err != nil {
		return manifest, err
	}
	for _, part := range manifest.Parts {
		h, err := tr.Next()
		if err != nil || h.Name != part.Module+".payload" || h.Typeflag != tar.TypeReg || h.Size != part.Size {
			return manifest, ErrInvalid
		}
		f, err := os.OpenFile(filepath.Join(directory, h.Name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return manifest, err
		}
		created = append(created, f.Name())
		hash := sha256.New()
		_, err = io.Copy(io.MultiWriter(f, hash), tr)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil || closeErr != nil || hex.EncodeToString(hash.Sum(nil)) != part.SHA256 {
			return manifest, ErrInvalid
		}
	}
	if _, err := tr.Next(); err != io.EOF {
		return manifest, ErrInvalid
	}
	// tar permits trailing zero blocks, but never a second archive or data.
	tail := make([]byte, 4096)
	for {
		n, err := plain.Read(tail)
		for _, b := range tail[:n] {
			if b != 0 {
				return manifest, ErrInvalid
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return manifest, err
		}
	}
	return manifest, nil
}

func Decode(data []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrInvalid
	}
	return nil
}

func OpenRegular(path string, limit int64) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > limit {
		return nil, ErrInvalid
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || !after.Mode().IsRegular() {
		f.Close()
		return nil, ErrInvalid
	}
	return f, nil
}

// Frame lengths and KDF work are fixed by version, never supplied by the file.
// A random 16-byte nonce prefix plus monotonic frame number prevents reuse;
// binding the entire header and index rejects reordering and header tampering.
func encrypt(w io.Writer, r io.Reader, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	header := append([]byte{}, magic...)
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	header = append(header, random...)
	key, err := scrypt.Key([]byte(password), random[:16], 32768, 8, 1, 32)
	if err != nil {
		return err
	}
	defer clear(key)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return err
	}
	if _, err := w.Write(header); err != nil {
		return err
	}
	buf := make([]byte, chunkSize)
	defer clear(buf)
	var total int64
	for index := uint64(0); ; index++ {
		n, readErr := io.ReadFull(r, buf)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return readErr
		}
		total += int64(n)
		if total > MaxBytes+1<<20 {
			return ErrInvalid
		}
		nonce := make([]byte, 24)
		copy(nonce, random[16:])
		binary.BigEndian.PutUint64(nonce[16:], index)
		aad := append(append([]byte{}, header...), nonce[16:]...)
		sealed := aead.Seal(nil, nonce, buf[:n], aad)
		if err := binary.Write(w, binary.BigEndian, uint32(n)); err != nil {
			return err
		}
		if _, err := w.Write(sealed); err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
}

func decrypt(w io.Writer, r io.Reader, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	header := make([]byte, len(magic)+32)
	if _, err := io.ReadFull(r, header); err != nil || !bytes.Equal(header[:len(magic)], magic) {
		return ErrInvalid
	}
	random := header[len(magic):]
	key, err := scrypt.Key([]byte(password), random[:16], 32768, 8, 1, 32)
	if err != nil {
		return err
	}
	defer clear(key)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return err
	}
	var total int64
	shortFrame := false
	for index := uint64(0); ; index++ {
		var n uint32
		if binary.Read(r, binary.BigEndian, &n) != nil || n > chunkSize || shortFrame && n != 0 {
			return ErrInvalid
		}
		shortFrame = n < chunkSize
		total += int64(n)
		if total > MaxBytes+1<<20 {
			return ErrInvalid
		}
		sealed := make([]byte, int(n)+aead.Overhead())
		if _, err := io.ReadFull(r, sealed); err != nil {
			return ErrInvalid
		}
		nonce := make([]byte, 24)
		copy(nonce, random[16:])
		binary.BigEndian.PutUint64(nonce[16:], index)
		aad := append(append([]byte{}, header...), nonce[16:]...)
		plain, err := aead.Open(nil, nonce, sealed, aad)
		if err != nil {
			return ErrInvalid
		}
		if n == 0 {
			var extra [1]byte
			n, err := r.Read(extra[:])
			if n != 0 || err != io.EOF {
				return ErrInvalid
			}
			return nil
		}
		_, err = w.Write(plain)
		clear(plain)
		if err != nil {
			return fmt.Errorf("write backup staging data: %w", err)
		}
	}
}
