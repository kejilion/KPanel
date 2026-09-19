package panel

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
)

// managedOperation is a versioned, typed operation, not an HTTP proxy. Its
// schema and validator are used both before approval and before dispatch.
// Cluster targets resolve the same name locally and never accept caller paths.
type managedOperation struct {
	Name, Domain, Description string
	ReadOnly                  bool
	Schema                    map[string]any
	resolved                  *jsonschema.Resolved
	Prepare                   func(json.RawMessage, *mcpaccess.Policy) (managedRequest, error)
}

type managedRequest struct {
	Method, Path, Query string
	Body                []byte
}

var managedReadPaths = map[string]string{
	"host_system_summary": "/v1/system/summary", "host_system_processes": "/v1/system/processes", "host_public_network": "/v1/system/public-network",
	"host_sites_list": "/v1/sites", "host_apps_list": "/v1/apps", "host_diagnostics_list": "/v1/diagnostics", "host_nginx_test": "/v1/nginx/test",
	"host_docker_summary": "/v1/docker/summary", "host_docker_containers": "/v1/docker/containers", "host_docker_resource_usage": "/v1/docker/container-stats",
	"host_docker_images": "/v1/docker/images", "host_docker_networks": "/v1/docker/networks", "host_docker_volumes": "/v1/docker/volumes",
	"host_docker_backups": "/v1/docker/backups", "host_docker_environment": "/v1/docker/environment", "host_docker_jobs": "/v1/docker/jobs",
	"host_compose_projects": "/v1/docker/compose-projects", "host_web_environment": "/v1/web-environment", "host_web_catalog": "/v1/web-environment/catalog",
	"host_web_backups": "/v1/web-environment/backups", "host_app_jobs": "/v1/app-jobs", "host_web_jobs": "/v1/web-environment/jobs",
}

func managedDomain(name string) string {
	switch {
	case strings.HasPrefix(name, "host_docker_"), strings.HasPrefix(name, "host_compose_"):
		return "docker"
	case strings.HasPrefix(name, "host_file_"):
		return "files"
	case strings.HasPrefix(name, "host_site"), strings.HasPrefix(name, "host_nginx_"), strings.HasPrefix(name, "host_web_"):
		return "sites"
	case strings.HasPrefix(name, "host_app_") || name == "host_apps_list":
		return "apps"
	case strings.HasPrefix(name, "host_diagnostic"):
		return "diagnostics"
	case strings.HasPrefix(name, "host_backup"):
		return "backups"
	default:
		return "system"
	}
}

var managedCatalog = sync.OnceValue(func() map[string]managedOperation {
	result := map[string]managedOperation{}
	add := func(name, description string, readOnly bool, raw []byte) {
		var schema map[string]any
		if json.Unmarshal(raw, &schema) != nil {
			panic("invalid managed operation schema")
		}
		properties := schema["properties"].(map[string]any)
		delete(properties, "reason")
		freezeManagedSchema(name, schema)
		// File contents are bounded consistently across HTTP, approval storage,
		// and the encrypted cluster envelope. Full files remain in File Manager.
		if name == "host_file_write" {
			properties["content"].(map[string]any)["maxLength"] = 16384
		}
		encoded, _ := json.Marshal(schema)
		var definition jsonschema.Schema
		if json.Unmarshal(encoded, &definition) != nil {
			panic("invalid managed operation schema")
		}
		resolved, err := definition.Resolve(nil)
		if err != nil {
			panic(err)
		}
		result[name] = managedOperation{Name: name, Domain: managedDomain(name), Description: description, ReadOnly: readOnly, Schema: schema, resolved: resolved}
	}
	for _, definition := range (&panelAITools{}).Definitions() {
		// Input belongs to each job owner's domain, not a universal job writer.
		if definition.Name == "host_job_input" || definition.Name == "host_site_change" {
			continue
		}
		add(definition.Name, definition.Description, definition.ReadOnly, definition.Schema)
	}
	for name := range managedReadPaths {
		if _, ok := result[name]; !ok {
			add(name, "Read current KPanel resources and resource versions. Results are redacted and paginated.", true, []byte(`{"type":"object","properties":{},"additionalProperties":false}`))
		}
	}
	addManagedExtensions(result, add)
	addManagedBackups(result, add)
	addManagedFiles(result, add)
	return result
})

func (op managedOperation) validate(raw json.RawMessage) error {
	if len(raw) > mcpaccess.MaxOperationArguments || !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		return errors.New("invalid_arguments")
	}
	var value any
	if json.Unmarshal(raw, &value) != nil || op.resolved.Validate(value) != nil {
		return errors.New("invalid_arguments")
	}
	return nil
}

func (op managedOperation) signature() string {
	data, _ := json.Marshal([]any{1, op.Name, op.Domain, op.ReadOnly, op.Schema})
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (op managedOperation) allowed(policy *mcpaccess.Policy) bool {
	return policy.AllowsOperation(op.Name, op.Domain, !op.ReadOnly) && policy.OperationVersions[op.Name] == op.signature()
}

func (op managedOperation) prepare(raw json.RawMessage, policy *mcpaccess.Policy) (managedRequest, error) {
	if err := op.validate(raw); err != nil {
		return managedRequest{}, err
	}
	if !op.allowed(policy) {
		return managedRequest{}, errors.New("operation_not_authorized")
	}
	if op.Domain == "files" {
		var input struct {
			Path    string
			Sources []string
		}
		_ = json.Unmarshal(raw, &input)
		if input.Path != "" && !policy.AllowsPath(input.Path) {
			return managedRequest{}, errors.New("file_root_not_authorized")
		}
		for _, source := range input.Sources {
			if !policy.AllowsPath(source) {
				return managedRequest{}, errors.New("file_root_not_authorized")
			}
		}
	}
	if op.Prepare != nil {
		return op.Prepare(raw, policy)
	}
	if op.ReadOnly {
		if path := managedReadPaths[op.Name]; path != "" {
			return managedRequest{Method: http.MethodGet, Path: path}, nil
		}
		if op.Name == "host_system_storage_usage" {
			var input struct{ Path string }
			_ = json.Unmarshal(raw, &input)
			if !aiStorageRoot(input.Path) {
				return managedRequest{}, errors.New("invalid_arguments")
			}
			return managedRequest{Method: http.MethodGet, Path: "/v1/system/storage-usage", Query: url.Values{"path": {input.Path}}.Encode()}, nil
		}
		if strings.HasPrefix(op.Name, "host_file_") {
			var input struct {
				Path, Search            string
				Limit, MaxBytes, Offset int
			}
			_ = json.Unmarshal(raw, &input)
			if op.Name != "host_file_list" && !aiFileReadable(input.Path) {
				return managedRequest{}, errors.New("file_content_protected")
			}
			query := url.Values{"path": {input.Path}}
			path := "/v1/files/text"
			switch op.Name {
			case "host_file_list":
				path = "/v1/files"
				if input.Limit == 0 {
					input.Limit = 50
				}
				query.Set("limit", fmt.Sprint(input.Limit))
				query.Set("offset", fmt.Sprint(input.Offset))
				if input.Search != "" {
					query.Set("search", input.Search)
				}
			case "host_file_tail":
				path = "/v1/files/tail"
				if input.MaxBytes == 0 {
					input.MaxBytes = 16384
				}
				if input.MaxBytes > 16384 {
					return managedRequest{}, errors.New("file_output_limit_16384")
				}
				query.Set("maxBytes", fmt.Sprint(input.MaxBytes))
			}
			return managedRequest{Method: http.MethodGet, Path: path, Query: query.Encode()}, nil
		}
		return managedRequest{}, errors.New("unsupported_operation")
	}
	if op.Name == "host_file_write" {
		var input struct{ Content string }
		_ = json.Unmarshal(raw, &input)
		if len(input.Content) > 16384 {
			return managedRequest{}, errors.New("file_write_limit_16384")
		}
	}
	method, path, target, body, err := (&panelAITools{}).prepareWrite(op.Name, raw)
	if err != nil {
		return managedRequest{}, errors.New("invalid_arguments")
	}
	query := ""
	if op.Name == "host_file_write" {
		query = url.Values{"path": {target}}.Encode()
	}
	return managedRequest{Method: method, Path: path, Query: query, Body: body}, nil
}

func (op managedOperation) automatic(raw json.RawMessage) bool {
	if op.Name == "host_diagnostic_start" {
		return true
	}
	if op.Name == "host_docker_container_action" || op.Name == "host_app_action" {
		var input struct{ Action string }
		_ = json.Unmarshal(raw, &input)
		return slices.Contains([]string{"start", "stop", "restart"}, input.Action)
	}
	return false
}

// A preset is expanded once into exact operation IDs. Adding a registry entry
// after an upgrade cannot expand any previously issued credential.
func managedPolicy(domains []string, write, automatic bool, roots []string) (*mcpaccess.Policy, error) {
	policy := &mcpaccess.Policy{Domains: slices.Clone(domains), Write: write, AutoApprove: automatic, FileRoots: slices.Clone(roots), OperationVersions: make(map[string]string)}
	for name, op := range managedCatalog() {
		if policy.Allows(op.Domain, !op.ReadOnly) {
			policy.Operations = append(policy.Operations, name)
			policy.OperationVersions[name] = op.signature()
		}
	}
	slices.Sort(policy.Operations)
	if !policy.Valid() {
		return nil, errors.New("invalid_mcp_access")
	}
	return policy, nil
}
