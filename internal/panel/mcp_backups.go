package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
)

type managedBackupExport struct {
	Password      string   `json:"password"`
	Modules       []string `json:"modules"`
	AgentRevision string   `json:"agentRevision,omitempty"`
}
type managedBackupID struct {
	BackupID string `json:"backupId"`
}
type managedBackupRestore struct {
	BackupID      string   `json:"backupId"`
	Modules       []string `json:"modules"`
	Revision      string   `json:"revision"`
	AgentRevision string   `json:"agentRevision,omitempty"`
}

func addManagedBackups(result map[string]managedOperation, add func(string, string, bool, []byte)) {
	register := func(name, description string, readOnly bool, schema []byte, prepare func(json.RawMessage, *mcpaccess.Policy) (managedRequest, error)) {
		add(name, description, readOnly, schema)
		op := result[name]
		op.Prepare = prepare
		result[name] = op
	}
	for _, item := range []struct{ Name, Path, Description string }{
		{"host_backups_list", "/api/v1/backups", "List existing encrypted backup exports, imported backup packages and restore jobs. Binary package upload/download uses the authenticated Panel backup page; do not put packages or passwords in URLs."},
		{"host_backup_inventory", "/api/v1/backups/inventory", "Read available backup modules and current target revisions before exporting or restoring."},
	} {
		register(item.Name, item.Description, true, []byte(`{"type":"object","properties":{},"additionalProperties":false}`), func(json.RawMessage, *mcpaccess.Policy) (managedRequest, error) {
			return managedRequest{Method: http.MethodGet, Path: item.Path}, nil
		})
	}
	register("host_backup_export", "Create an encrypted backup of selected existing modules. Requires Panel approval. Returns the actual backup job; download its package from the authenticated Panel backup page.", false, managedSchema[managedBackupExport](), func(raw json.RawMessage, _ *mcpaccess.Policy) (managedRequest, error) {
		var input managedBackupExport
		if decodeStrictToolArguments(raw, &input) != nil || backup.ValidatePassword(input.Password) != nil {
			return managedRequest{}, errors.New("invalid_backup_request")
		}
		if _, err := backup.Selection(input.Modules); err != nil {
			return managedRequest{}, errors.New("invalid_backup_modules")
		}
		body, _ := json.Marshal(backupRequest{Password: input.Password, Modules: input.Modules, AgentRevision: input.AgentRevision})
		return managedRequest{Method: http.MethodPost, Path: "/api/v1/backups/export", Body: body}, nil
	})
	for _, item := range []struct{ Name, Method, Suffix, Description string }{
		{"host_backup_job", http.MethodGet, "", "Read a backup or restore job by backupId. After restoring Panel credentials, reauthorize in Panel before reconnecting."},
		{"host_backup_preview", http.MethodPost, "/preview", "Refresh restore preflight for an already uploaded and validated package. Requires approval; returns targetRevision and agentRevision needed for restoration."},
		{"host_backup_recover", http.MethodPost, "/recover", "Ask the existing backup service to recover a failed host restore. Requires approval; inspect its real job result."},
		{"host_backup_delete", http.MethodDelete, "", "Delete an existing backup record and package. Requires approval."},
	} {
		register(item.Name, item.Description, item.Method == http.MethodGet, managedSchema[managedBackupID](), func(raw json.RawMessage, _ *mcpaccess.Policy) (managedRequest, error) {
			var input managedBackupID
			if decodeStrictToolArguments(raw, &input) != nil || !backup.ValidID(input.BackupID) {
				return managedRequest{}, errors.New("invalid_backup_id")
			}
			return managedRequest{Method: item.Method, Path: "/api/v1/backups/" + input.BackupID + item.Suffix, Body: []byte(`{}`)}, nil
		})
	}
	register("host_backup_restore", "Restore selected modules from an imported backup using the fresh preview revisions. Always requires approval. Restoring Panel data may restart the service and revoke MCP credentials; verify the restore in Panel and reauthorize before reconnecting.", false, managedSchema[managedBackupRestore](), func(raw json.RawMessage, _ *mcpaccess.Policy) (managedRequest, error) {
		var input managedBackupRestore
		if decodeStrictToolArguments(raw, &input) != nil || !backup.ValidID(input.BackupID) || input.Revision == "" || len(input.Revision) > 128 || len(input.AgentRevision) > 128 {
			return managedRequest{}, errors.New("invalid_backup_restore")
		}
		if _, err := backup.Selection(input.Modules); err != nil {
			return managedRequest{}, errors.New("invalid_backup_modules")
		}
		body, _ := json.Marshal(backupRequest{Modules: input.Modules, Revision: input.Revision, AgentRevision: input.AgentRevision})
		return managedRequest{Method: http.MethodPost, Path: "/api/v1/backups/" + input.BackupID + "/restore", Body: body}, nil
	})
}

func (s *Server) mcpLocalRequest(ctx context.Context, op managedOperation, prepared managedRequest, requestID string) (AgentResponse, error) {
	if op.Domain != "backups" {
		if prepared.Method == http.MethodGet {
			return s.hostOps.Get(ctx, prepared.Path, prepared.Query, requestID)
		}
		return s.hostOps.Do(ctx, prepared.Method, prepared.Path, prepared.Query, requestID, prepared.Body)
	}
	request, err := http.NewRequestWithContext(ctx, prepared.Method, prepared.Path, bytes.NewReader(prepared.Body))
	if err != nil {
		return AgentResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response := &mcpResponseBuffer{header: make(http.Header)}
	s.serveBackupOperation(response, request)
	if response.overflow {
		return AgentResponse{}, errors.New("backup_response_too_large")
	}
	status := response.status
	if status == 0 {
		status = http.StatusOK
	}
	return AgentResponse{StatusCode: status, Body: response.body.Bytes()}, nil
}
