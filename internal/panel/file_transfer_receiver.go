package panel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

var errReceiveUnsupported = errors.New("target does not support receiving sessions")

type fileReceiveHTTPError struct {
	status int
	code   string
}

func (e *fileReceiveHTTPError) Error() string {
	return fmt.Sprintf("file receiver HTTP %d (%s)", e.status, e.code)
}

type fileReceiveTarget struct {
	open     func(context.Context, string, string, io.Reader, int64) (*http.Response, error)
	supports func(context.Context, contract.FileReceiveInput) error
}

func (s *Server) fileReceiver(hostID string, kind cluster.HostKind, requestID string) fileReceiveTarget {
	return fileReceiveTarget{supports: func(ctx context.Context, input contract.FileReceiveInput) error {
		if hostID == "" {
			return nil // The local Agent has an ordinary 404 compatibility signal.
		}
		return s.fileReceiveSupport(ctx, hostID, kind, input.Directory, requestID)
	}, open: func(ctx context.Context, method, query string, body io.Reader, size int64) (*http.Response, error) {
		mediaType := "application/json"
		if method == http.MethodPut {
			mediaType = "application/octet-stream"
		}
		if hostID != "" {
			return s.openFileHostRequest(ctx, hostID, kind, cluster.LightFileRequest{
				Method: method, Path: "/v1/files/transfer/sessions", RawQuery: query,
				Headers: map[string]string{"Content-Type": mediaType}, Body: body, BodyLength: size,
			})
		}
		streamer, ok := s.agent.(agentStreamAPI)
		if !ok {
			return nil, errors.New("Agent stream unavailable")
		}
		return streamer.OpenStream(ctx, method, "/v1/files/transfer/sessions", query, requestID, body, http.Header{"Content-Type": []string{mediaType}}, size)
	}}
}

func (s *Server) fileReceiveSupport(ctx context.Context, hostID string, kind cluster.HostKind, directory, requestID string) error {
	query := url.Values{"path": {directory}, "limit": {"1"}}.Encode()
	var payload []byte
	if hostID == "" {
		response, err := s.agent.Get(ctx, "/v1/files", query, requestID)
		if err != nil {
			return err
		}
		if response.StatusCode != http.StatusOK {
			return &fileReceiveHTTPError{status: response.StatusCode}
		}
		payload = response.Body
	} else {
		input := cluster.LightFileRequest{
			Method: http.MethodGet, Path: "/v1/files", RawQuery: query, Body: http.NoBody,
		}
		var response *http.Response
		var err error
		if kind == cluster.HostKindPanel {
			host, hostErr := s.cluster.Host(ctx, hostID)
			if hostErr != nil {
				return hostErr
			}
			if host.FederationProtocol == cluster.FederationProtocol {
				response, err = s.cluster.OpenRemotePanelFileV1(ctx, hostID, input)
			} else {
				response, err = s.openFileHostRequest(ctx, hostID, kind, input)
			}
		} else {
			response, err = s.openFileHostRequest(ctx, hostID, kind, input)
		}
		if err != nil {
			return err
		}
		defer response.Body.Close()
		stop := context.AfterFunc(ctx, func() { _ = response.Body.Close() })
		defer stop()
		if response.StatusCode != http.StatusOK {
			return &fileReceiveHTTPError{status: response.StatusCode}
		}
		payload, err = io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
		if err != nil {
			return err
		}
	}
	var result contract.FileDirectory
	if len(payload) > 64<<10 || json.Unmarshal(payload, &result) != nil || result.Path != directory ||
		result.FileReceiveVersion < 0 || result.FileReceiveVersion > 1 || result.PanelFileReceiveVersion < 0 || result.PanelFileReceiveVersion > 1 {
		return errors.New("receiver capability response invalid")
	}
	if result.FileReceiveVersion != 1 || hostID != "" && kind == cluster.HostKindPanel && result.PanelFileReceiveVersion != 1 {
		return errReceiveUnsupported
	}
	return nil
}

func readReceiveResponse(ctx context.Context, response *http.Response, err error) (contract.FileReceiveSession, error) {
	if err != nil {
		return contract.FileReceiveSession{}, err
	}
	if response == nil || response.Body == nil {
		return contract.FileReceiveSession{}, errors.New("receiver result missing")
	}
	defer response.Body.Close()
	stop := context.AfterFunc(ctx, func() { _ = response.Body.Close() })
	defer stop()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var problem struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&problem)
		if response.StatusCode == http.StatusNotFound && (problem.Code == "route_not_found" || problem.Code == "not_found" || problem.Code == "") {
			return contract.FileReceiveSession{}, errReceiveUnsupported
		}
		return contract.FileReceiveSession{}, &fileReceiveHTTPError{response.StatusCode, problem.Code}
	}
	var session contract.FileReceiveSession
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64<<10))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&session) != nil || decoder.Decode(&extra) != io.EOF ||
		len(session.ID) != 64 || len(session.PrefixSHA256) != 64 || session.Offset < 0 ||
		session.ChunkBytes != contract.FileTransferChunkBytes ||
		(session.State != "receiving" && session.State != "committing" && session.State != "complete") {
		return contract.FileReceiveSession{}, errors.New("receiver result invalid")
	}
	return session, nil
}

func (t fileReceiveTarget) command(ctx context.Context, input contract.FileReceiveRequest) (contract.FileReceiveSession, error) {
	if input.Operation == "create" && input.Input != nil && t.supports != nil {
		if err := t.supports(ctx, *input.Input); err != nil {
			return contract.FileReceiveSession{}, err
		}
	}
	data, err := json.Marshal(input)
	if err != nil {
		return contract.FileReceiveSession{}, err
	}
	return t.send(ctx, http.MethodPost, "", bytes.NewReader(data), int64(len(data)))
}

func (t fileReceiveTarget) send(ctx context.Context, method, query string, body io.Reader, length int64) (contract.FileReceiveSession, error) {
	response, err := t.open(ctx, method, query, body, length)
	return readReceiveResponse(ctx, response, err)
}

func (t fileReceiveTarget) status(ctx context.Context, id, key string) (contract.FileReceiveSession, error) {
	query := url.Values{"id": {id}, "sourceKey": {key}}
	return t.send(ctx, http.MethodGet, query.Encode(), http.NoBody, 0)
}

func retryFileReceive(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var remote *fileReceiveHTTPError
	if errors.As(err, &remote) {
		return remote.status == 429 || remote.status == 502 || remote.status == 503 || remote.status == 504
	}
	return !errors.Is(err, errReceiveUnsupported)
}

func transferRetryPause(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Every source adapter uses the same durable acknowledgements, retry policy,
// end verification and atomic commit. Source identity/permissions stay in the
// adapter, and retries never publish a request's partially read chunk.
func (s *Server) receiveFileTransfer(
	ctx context.Context, input contract.FileReceiveInput, initial io.ReadCloser,
	reopen func(context.Context, int64, string) (io.ReadCloser, error), target fileReceiveTarget,
	emit func(contract.FileTransferEvent) bool,
	legacy func(io.ReadCloser) (contract.FileEntry, error),
) (contract.FileEntry, int64, error) {
	body := initial
	defer func() {
		if body != nil {
			_ = body.Close()
		}
	}()
	stop := context.AfterFunc(ctx, func() { _ = initial.Close() })
	defer func() { stop() }()
	session, err := target.command(ctx, contract.FileReceiveRequest{Operation: "create", Input: &input})
	if errors.Is(err, errReceiveUnsupported) && legacy != nil {
		entry, err := legacy(body)
		return entry, entry.SizeBytes, err
	}
	if err != nil {
		return contract.FileEntry{}, 0, err
	}
	id := session.ID
	defer s.abortFileReceive(id, input.SourceKey, target)
	if session.Offset != 0 || session.State != "receiving" {
		return contract.FileEntry{}, 0, errors.New("receiver creation invalid")
	}
	buffer := make([]byte, contract.FileTransferChunkBytes)
	hasher := sha256.New()
	reader := &fileTransferEOFReader{Reader: body}
	reconnects := 0
	for {
		if err := ctx.Err(); err != nil {
			return contract.FileEntry{}, session.Offset, err
		}
		count, readErr := io.ReadFull(reader, buffer)
		ended := readErr != nil && reader.eof && (readErr == io.EOF || readErr == io.ErrUnexpectedEOF)
		if readErr != nil && !ended {
			if reopen == nil || reconnects >= 3 {
				return contract.FileEntry{}, session.Offset, readErr
			}
			_ = body.Close()
			if err := transferRetryPause(ctx, reconnects); err != nil {
				return contract.FileEntry{}, session.Offset, err
			}
			reconnects++
			body, err = reopen(ctx, session.Offset, session.PrefixSHA256)
			if err != nil {
				return contract.FileEntry{}, session.Offset, err
			}
			stop()
			current := body
			stop = context.AfterFunc(ctx, func() { _ = current.Close() })
			reader = &fileTransferEOFReader{Reader: body}
			continue
		}
		if count > 0 {
			digest := sha256.Sum256(buffer[:count])
			query := url.Values{"id": {id}, "sourceKey": {input.SourceKey}, "offset": {strconv.FormatInt(session.Offset, 10)}, "sha256": {hex.EncodeToString(digest[:])}}
			before := session.Offset
			var next contract.FileReceiveSession
			for attempt := 0; attempt < 3; attempt++ {
				next, err = target.send(ctx, http.MethodPut, query.Encode(), bytes.NewReader(buffer[:count]), int64(count))
				if err == nil {
					break
				}
				if !retryFileReceive(err) || attempt == 2 {
					return contract.FileEntry{}, session.Offset, err
				}
				if err := transferRetryPause(ctx, attempt); err != nil {
					return contract.FileEntry{}, session.Offset, err
				}
			}
			_, _ = hasher.Write(buffer[:count])
			if next.ID != id || next.Offset != before+int64(count) || next.PrefixSHA256 != hex.EncodeToString(hasher.Sum(nil)) || next.State != "receiving" {
				return contract.FileEntry{}, before, errors.New("receiver checkpoint invalid")
			}
			session = next
			if !emit(contract.FileTransferEvent{State: "transferring", LoadedBytes: session.Offset, TotalBytes: max(input.SizeBytes, 0), Name: input.Name}) {
				return contract.FileEntry{}, session.Offset, context.Canceled
			}
		}
		if ended {
			break
		}
	}
	if !emit(contract.FileTransferEvent{State: "committing", LoadedBytes: session.Offset, TotalBytes: max(input.SizeBytes, 0), Name: input.Name}) {
		return contract.FileEntry{}, session.Offset, context.Canceled
	}
	received := session.Offset
	for attempt := 0; attempt < 3; attempt++ {
		session, err = target.command(ctx, contract.FileReceiveRequest{Operation: "commit", ID: id, SourceKey: input.SourceKey, SizeBytes: received, SHA256: hex.EncodeToString(hasher.Sum(nil))})
		if err == nil {
			break
		}
		if !retryFileReceive(err) || attempt == 2 {
			return contract.FileEntry{}, 0, err
		}
		if err := transferRetryPause(ctx, attempt); err != nil {
			return contract.FileEntry{}, 0, err
		}
		// A lost commit acknowledgement is recovered using the same session ID.
		session, err = target.status(ctx, id, input.SourceKey)
		if err == nil && session.State == "complete" {
			break
		}
		if err != nil {
			return contract.FileEntry{}, 0, err
		}
	}
	if session.State != "complete" || session.Entry == nil || session.Entry.Path != pathForReceive(input) ||
		session.Entry.Kind != input.Kind || input.Kind == "file" && session.Entry.SizeBytes != session.Offset {
		return contract.FileEntry{}, session.Offset, errors.New("receiver commit result invalid")
	}
	return *session.Entry, session.Offset, nil
}

func pathForReceive(input contract.FileReceiveInput) string {
	return path.Join(input.Directory, input.Name)
}

type fileTransferEOFReader struct {
	io.Reader
	eof bool
}

func (r *fileTransferEOFReader) Read(buffer []byte) (int, error) {
	n, err := r.Reader.Read(buffer)
	if err == io.EOF {
		r.eof = true
	}
	return n, err
}
