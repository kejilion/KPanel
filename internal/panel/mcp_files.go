package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
)

func intersectMCPFileRoots(target, client []string) []string {
	roots := []string{}
	for _, a := range target {
		if path.Clean(a) != a || a == "/" {
			continue
		}
		for _, b := range client {
			if path.Clean(b) != b || b == "/" {
				continue
			}
			candidate := ""
			if a == b || strings.HasPrefix(a, b+"/") {
				candidate = a
			} else if strings.HasPrefix(b, a+"/") {
				candidate = b
			}
			if candidate != "" && !slices.Contains(roots, candidate) {
				roots = append(roots, candidate)
			}
		}
	}
	slices.Sort(roots)
	return roots
}

type managedTrashAction struct {
	Action                   string            `json:"action"`
	TrashIDs                 []string          `json:"trashIds"`
	ExpectedResourceVersions map[string]string `json:"expectedResourceVersions"`
}

func addManagedFiles(result map[string]managedOperation, add func(string, string, bool, []byte)) {
	add("host_file_trash_list", "List trash entries whose original paths are inside the granted file roots. Entries without a verifiable original path are omitted. Use pageOffset/pageSize; scanTruncated means File Manager's bounded inventory is incomplete.", true, []byte(`{"type":"object","properties":{},"additionalProperties":false}`))
	op := result["host_file_trash_list"]
	op.Prepare = func(json.RawMessage, *mcpaccess.Policy) (managedRequest, error) {
		return managedRequest{Method: http.MethodGet, Path: "/v1/files/trash"}, nil
	}
	result[op.Name] = op
	add("host_file_trash_action", "Restore or permanently delete selected trash entries. Read their resource versions first. Only entries with original paths inside the granted roots may be changed. Requires approval; bulk emptying is not exposed.", false, managedSchema[managedTrashAction]())
	op = result["host_file_trash_action"]
	op.Prepare = func(raw json.RawMessage, _ *mcpaccess.Policy) (managedRequest, error) {
		var input managedTrashAction
		if decodeStrictToolArguments(raw, &input) != nil || len(input.TrashIDs) == 0 || len(input.TrashIDs) > 20 || len(input.ExpectedResourceVersions) != len(input.TrashIDs) {
			return managedRequest{}, errors.New("invalid_trash_selection")
		}
		seen := map[string]bool{}
		for _, id := range input.TrashIDs {
			if id == "" || len(id) > 255 || strings.ContainsAny(id, "/\\\x00") || seen[id] || input.ExpectedResourceVersions[id] == "" || len(input.ExpectedResourceVersions[id]) > 256 {
				return managedRequest{}, errors.New("invalid_trash_selection")
			}
			seen[id] = true
		}
		return managedRequest{Method: http.MethodPost, Path: "/v1/files/actions", Body: raw}, nil
	}
	result[op.Name] = op
}

func mcpFileReadProjection(tool string, body []byte, policy *mcpaccess.Policy) ([]byte, error) {
	if tool == "host_file_tail" && !validAIFileTailProjection(body) {
		return nil, errors.New("file_projection_unavailable")
	}
	if tool != "host_file_trash_list" {
		return body, nil
	}
	var directory contract.FileTrashDirectory
	if json.Unmarshal(body, &directory) != nil {
		return nil, errors.New("invalid_trash_inventory")
	}
	items := []contract.FileTrashEntry{}
	for _, entry := range directory.Entries {
		if policy.AllowsPath(entry.OriginalPath) && aiFileMutable(entry.OriginalPath) {
			items = append(items, entry)
		}
	}
	// Counts and pagination must describe only the authorized subset.
	return json.Marshal(map[string]any{"items": items, "readAt": directory.ReadAt, "scanTruncated": directory.Truncated})
}

func strictProjectionObject(body []byte, allowed map[string]bool) (map[string]json.RawMessage, bool) {
	if len(body) > 512<<10 {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, false
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || !allowed[key] {
			return nil, false
		}
		if _, exists := fields[key]; exists {
			return nil, false
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, false
		}
		fields[key] = value
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || decoder.Decode(&struct{}{}) != io.EOF {
		return nil, false
	}
	return fields, true
}

func validAIFileTailProjection(body []byte) bool {
	fields, ok := strictProjectionObject(body, map[string]bool{"path": true, "content": true, "sizeBytes": true, "resourceVersion": true, "truncated": true, "projection": true})
	if !ok || len(fields) != 6 {
		return false
	}
	var proof struct {
		Path            *string `json:"path"`
		Content         *string `json:"content"`
		SizeBytes       *int64  `json:"sizeBytes"`
		ResourceVersion *string `json:"resourceVersion"`
		Truncated       *bool   `json:"truncated"`
		Projection      string  `json:"projection"`
	}
	return json.Unmarshal(body, &proof) == nil && proof.Path != nil && *proof.Path != "" && proof.Content != nil && proof.SizeBytes != nil && *proof.SizeBytes >= 0 && proof.ResourceVersion != nil && resourceVersionPattern.MatchString(*proof.ResourceVersion) && proof.Truncated != nil && proof.Projection == "full-context-v1"
}

func validRemoteAIFileTailProjection(body []byte) bool {
	fields, ok := strictProjectionObject(body, map[string]bool{"data": true, "observedAt": true, "redacted": true, "truncated": true})
	if !ok || len(fields) != 4 || !validAIFileTailProjection(fields["data"]) {
		return false
	}
	var envelope struct {
		ObservedAt time.Time `json:"observedAt"`
		Redacted   bool      `json:"redacted"`
		Truncated  *bool     `json:"truncated"`
	}
	return json.Unmarshal(body, &envelope) == nil && !envelope.ObservedAt.IsZero() && envelope.Redacted && envelope.Truncated != nil
}

func (s *Server) mcpTrashPreflight(ctx context.Context, raw json.RawMessage, policy *mcpaccess.Policy) error {
	var input managedTrashAction
	if json.Unmarshal(raw, &input) != nil {
		return errors.New("invalid_trash_selection")
	}
	response, err := s.hostOps.Get(ctx, "/v1/files/trash", "", newRequestID())
	if err != nil || response.StatusCode != 200 {
		return errors.New("trash_inventory_unavailable")
	}
	var directory contract.FileTrashDirectory
	if json.Unmarshal(response.Body, &directory) != nil {
		return errors.New("invalid_trash_inventory")
	}
	allowed := map[string]string{}
	for _, entry := range directory.Entries {
		if policy.AllowsPath(entry.OriginalPath) && aiFileMutable(entry.OriginalPath) {
			allowed[entry.ID] = entry.ResourceVersion
		}
	}
	for _, id := range input.TrashIDs {
		if allowed[id] == "" || allowed[id] != input.ExpectedResourceVersions[id] {
			return errors.New("trash_entry_unavailable_or_version_changed")
		}
	}
	return nil
}
