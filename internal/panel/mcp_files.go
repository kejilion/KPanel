package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
)

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
