package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
	"github.com/kejilion/kejilion-panel/internal/sites"
	"github.com/kejilion/kejilion-panel/internal/webenv"
)

func managedSchema[T any]() []byte {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	data, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	return data
}

func managedTypedRequest[T any](method, path string, validate func(*T) (string, string)) func(json.RawMessage, *mcpaccess.Policy) (managedRequest, error) {
	return func(raw json.RawMessage, _ *mcpaccess.Policy) (managedRequest, error) {
		var input T
		if decodeStrictToolArguments(raw, &input) != nil {
			return managedRequest{}, errors.New("invalid_arguments")
		}
		if validate != nil {
			if field, _ := validate(&input); field != "" {
				return managedRequest{}, errors.New("invalid_arguments")
			}
		}
		body, err := json.Marshal(input)
		return managedRequest{Method: method, Path: path, Body: body}, err
	}
}

type managedSiteUpdate struct {
	SiteID string                `json:"siteId"`
	Site   sites.ScriptSiteInput `json:"site"`
}
type managedSiteDelete struct {
	SiteID                  string `json:"siteId"`
	PrimaryDomain           string `json:"primaryDomain"`
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
}
type managedJobQuery struct {
	JobID  string `json:"jobId"`
	Output bool   `json:"output,omitempty"`
	Offset int64  `json:"offset,omitempty"`
}
type managedJobInput struct {
	JobID string `json:"jobId"`
	Data  string `json:"data"`
}
type managedJobCancel struct {
	JobID string `json:"jobId"`
}
type managedSystemDetails struct {
	Section string `json:"section"`
}
type managedSystemLogs struct {
	Source   string `json:"source"`
	Limit    int    `json:"limit,omitempty"`
	Priority string `json:"priority,omitempty"`
}
type managedComposeDetail struct {
	Name string `json:"name"`
}

var managedSystemSections = map[string]string{
	"runtime": "/v1/system/runtime", "configuration": "/v1/system/management/config", "bbr": "/v1/system/management/bbrv3",
	"hosts": "/v1/system/hosts", "cron": "/v1/system/cron", "interfaces": "/v1/system/network-interfaces", "firewall": "/v1/system/firewall",
	"ports": "/v1/system/port-usage", "traffic_shutdown": "/v1/system/traffic-shutdown", "accounts": "/v1/system/accounts",
	"ssh_defense": "/v1/system/ssh-defense", "tuning": "/v1/system/system-tuning", "partitions": "/v1/system/disk-partitions", "log_usage": "/v1/system/logs/summary",
}

type managedJobOwner struct {
	Domain, Path          string
	Output, Input, Cancel bool
}

var managedJobSources = map[string]managedJobOwner{
	"docker":       {"docker", "/v1/docker/jobs", false, false, false},
	"app":          {"apps", "/v1/app-jobs", true, true, true},
	"site":         {"sites", "/v1/site-installations", true, true, false},
	"web":          {"sites", "/v1/web-environment/jobs", true, true, false},
	"diagnostic":   {"diagnostics", "/v1/diagnostic-jobs", true, true, false},
	"file_archive": {"files", "/v1/files/archive-jobs", false, false, true},
}

func addManagedExtensions(result map[string]managedOperation, add func(string, string, bool, []byte)) {
	register := func(name, description string, readOnly bool, schema []byte, prepare func(json.RawMessage, *mcpaccess.Policy) (managedRequest, error)) {
		add(name, description, readOnly, schema)
		op := result[name]
		op.Prepare = prepare
		result[name] = op
	}
	register("host_system_details", "Read a fixed system resource: configuration, runtime, hosts, cron, interfaces, firewall, ports, traffic_shutdown, accounts, ssh_defense, tuning, partitions, log_usage or bbr.", true, managedSchema[managedSystemDetails](), func(raw json.RawMessage, _ *mcpaccess.Policy) (managedRequest, error) {
		var input managedSystemDetails
		_ = json.Unmarshal(raw, &input)
		path := managedSystemSections[input.Section]
		if path == "" {
			return managedRequest{}, errors.New("invalid_system_section")
		}
		return managedRequest{Method: http.MethodGet, Path: path}, nil
	})
	register("host_system_logs", "Read a bounded, redacted log window from system, journal, login or security. Use the returned truncation/partial flags.", true, managedSchema[managedSystemLogs](), func(raw json.RawMessage, _ *mcpaccess.Policy) (managedRequest, error) {
		var input managedSystemLogs
		_ = json.Unmarshal(raw, &input)
		if input.Limit == 0 {
			input.Limit = 50
		}
		if input.Limit > 100 {
			return managedRequest{}, errors.New("invalid_log_limit")
		}
		query := url.Values{"source": {input.Source}, "limit": {fmt.Sprint(input.Limit)}}
		if input.Priority != "" {
			query.Set("priority", input.Priority)
		}
		if _, field, _ := contract.ParseSystemLogQuery(query); field != "" {
			return managedRequest{}, errors.New("invalid_log_query")
		}
		return managedRequest{Method: http.MethodGet, Path: "/v1/system/logs", Query: query.Encode()}, nil
	})
	register("host_system_resource_action", "Manage hosts entries, cron, network interfaces and firewall through the existing validated system actions. Host changes require approval.", false, managedSchema[contract.SystemResourceActionRequest](), managedTypedRequest(http.MethodPost, "/v1/system/resource-actions", contract.ValidateSystemResourceAction))
	register("host_system_account_action", "Manage system accounts using explicit, validated account operations. Always requires approval.", false, managedSchema[contract.AccountManagementActionRequest](), managedTypedRequest(http.MethodPost, "/v1/system/account-actions", contract.ValidateAccountManagementAction))
	register("host_system_ssh_defense_action", "Manage SSH defense using the current resource version. Always requires approval.", false, managedSchema[contract.SSHDefenseActionRequest](), managedTypedRequest(http.MethodPost, "/v1/system/ssh-defense/actions", contract.ValidateSSHDefenseAction))
	register("host_system_tuning_action", "Apply selected fixed system tuning items with resource version validation. Always requires approval.", false, managedSchema[contract.SystemTuningActionRequest](), managedTypedRequest(http.MethodPost, "/v1/system/system-tuning/actions", contract.ValidateSystemTuningAction))
	register("host_system_traffic_action", "Configure traffic shutdown using existing validated actions. Always requires approval.", false, managedSchema[contract.TrafficShutdownActionRequest](), managedTypedRequest(http.MethodPost, "/v1/system/traffic-shutdown/actions", contract.ValidateTrafficShutdownAction))
	register("host_system_partition_action", "Manage disk partitions through fixed validated actions with current resource version. Destructive actions require Panel approval.", false, managedSchema[contract.DiskPartitionActionRequest](), managedTypedRequest(http.MethodPost, "/v1/system/disk-partition-actions", contract.ValidateDiskPartitionAction))
	register("host_web_action", "Install, maintain, back up or restore the managed web environment. Read host_web_catalog and host_web_environment first. Returns a task; always requires approval.", false, managedSchema[webenv.ActionRequest](), managedTypedRequest[webenv.ActionRequest](http.MethodPost, "/v1/web-environment/jobs", nil))
	register("host_site_create", "Create a site using a structured site definition, including the existing template and certificate workflows. Returns the site or an installation task.", false, managedSchema[sites.ScriptSiteInput](), managedTypedRequest[sites.ScriptSiteInput](http.MethodPost, "/v1/sites", nil))
	register("host_site_update", "Update an existing site or its certificate. Read the site first and supply its expectedResourceVersion in site.", false, managedSchema[managedSiteUpdate](), func(raw json.RawMessage, _ *mcpaccess.Policy) (managedRequest, error) {
		var input managedSiteUpdate
		if decodeStrictToolArguments(raw, &input) != nil || !siteIDPattern.MatchString(input.SiteID) || !resourceVersionPattern.MatchString(input.Site.ExpectedResourceVersion) {
			return managedRequest{}, errors.New("invalid_site_update")
		}
		body, err := json.Marshal(input.Site)
		return managedRequest{Method: http.MethodPatch, Path: "/v1/sites/" + input.SiteID, Body: body}, err
	})
	register("host_site_delete", "Delete a site after verifying its domain and resource version. Always requires Panel approval.", false, managedSchema[managedSiteDelete](), func(raw json.RawMessage, _ *mcpaccess.Policy) (managedRequest, error) {
		var input managedSiteDelete
		if decodeStrictToolArguments(raw, &input) != nil || !siteIDPattern.MatchString(input.SiteID) || !resourceVersionPattern.MatchString(input.ExpectedResourceVersion) || input.PrimaryDomain == "" {
			return managedRequest{}, errors.New("invalid_site_delete")
		}
		body, _ := json.Marshal(sites.DeleteInput{PrimaryDomain: input.PrimaryDomain, ExpectedResourceVersion: input.ExpectedResourceVersion})
		return managedRequest{Method: http.MethodDelete, Path: "/v1/sites/" + input.SiteID, Body: body}, nil
	})
	register("host_compose_detail", "Read a Compose project's current resource version and redacted definition. Never overwrite a definition using masked secret values.", true, managedSchema[managedComposeDetail](), func(raw json.RawMessage, _ *mcpaccess.Policy) (managedRequest, error) {
		var input managedComposeDetail
		_ = json.Unmarshal(raw, &input)
		if len(input.Name) == 0 || len(input.Name) > 128 || strings.ContainsAny(input.Name, "/\\\x00?#") || input.Name == "." || input.Name == ".." {
			return managedRequest{}, errors.New("invalid_compose_name")
		}
		return managedRequest{Method: http.MethodGet, Path: "/v1/docker/compose-projects/" + url.PathEscape(input.Name)}, nil
	})
	register("host_file_action", "Create directories, copy, move, rename, chmod, archive, extract or trash files within the granted roots. Uses File Manager protections and resource versions. Always requires approval.", false, managedSchema[contract.FileActionRequest](), func(raw json.RawMessage, policy *mcpaccess.Policy) (managedRequest, error) {
		var input contract.FileActionRequest
		if decodeStrictToolArguments(raw, &input) != nil || !slices.Contains([]string{"mkdir", "copy", "move", "rename", "chmod", "compress", "extract", "trash"}, input.Action) || len(input.Sources) > 20 || len(input.TrashIDs) > 0 {
			return managedRequest{}, errors.New("invalid_file_action")
		}
		if input.Target != "" && (!policy.AllowsPath(input.Target) || !aiFileMutable(input.Target)) {
			return managedRequest{}, errors.New("file_root_not_authorized")
		}
		for _, source := range input.Sources {
			if !policy.AllowsPath(source) || !aiFileMutable(source) {
				return managedRequest{}, errors.New("file_root_not_authorized")
			}
		}
		if len(input.Sources) == 0 && input.Target == "" {
			return managedRequest{}, errors.New("invalid_file_action")
		}
		if input.Action != "mkdir" {
			if len(input.Sources) == 0 {
				return managedRequest{}, errors.New("invalid_file_action")
			}
			for _, source := range input.Sources {
				version := input.ExpectedResourceVersions[source]
				if input.Action == "rename" || input.Action == "extract" {
					version = input.ExpectedResourceVersion
				}
				if !resourceVersionPattern.MatchString(version) {
					return managedRequest{}, errors.New("file_resource_version_required")
				}
			}
		}
		body, _ := json.Marshal(input)
		if input.Action == "compress" || input.Action == "extract" {
			body, _ = json.Marshal(contract.FileArchiveJobRequest{Operation: "create", Input: &input})
			return managedRequest{Method: http.MethodPost, Path: "/v1/files/archive-jobs", Body: body}, nil
		}
		return managedRequest{Method: http.MethodPost, Path: "/v1/files/actions", Body: body}, nil
	})
	for source, owner := range managedJobSources {
		name := "host_" + source + "_job"
		register(name, "Read a real background task by jobId. For interactive tasks, output=true reads a bounded output window at offset; the owner remains the source of truth.", true, managedSchema[managedJobQuery](), func(raw json.RawMessage, _ *mcpaccess.Policy) (managedRequest, error) {
			var input managedJobQuery
			_ = json.Unmarshal(raw, &input)
			if !ownerJobIDPattern.MatchString(input.JobID) || input.Offset < 0 || (input.Output && !owner.Output) {
				return managedRequest{}, errors.New("invalid_job_query")
			}
			path, query := owner.Path+"/"+input.JobID, ""
			if source == "file_archive" {
				path = owner.Path
				query = url.Values{"id": {input.JobID}}.Encode()
			}
			if input.Output {
				path += "/terminal"
				query = url.Values{"offset": {fmt.Sprint(input.Offset)}}.Encode()
			}
			return managedRequest{Method: http.MethodGet, Path: path, Query: query}, nil
		})
		op := result[name]
		op.Domain = owner.Domain
		result[name] = op
		if owner.Input {
			name := "host_" + source + "_job_input"
			register(name, "Submit input to an existing interactive task owned by this business domain. Requires approval; never starts a host shell.", false, managedSchema[managedJobInput](), func(raw json.RawMessage, _ *mcpaccess.Policy) (managedRequest, error) {
				var input managedJobInput
				_ = json.Unmarshal(raw, &input)
				if !ownerJobIDPattern.MatchString(input.JobID) || input.Data == "" || len(input.Data) > 16384 || strings.ContainsRune(input.Data, 0) {
					return managedRequest{}, errors.New("invalid_job_input")
				}
				body, _ := json.Marshal(map[string]string{"data": input.Data})
				return managedRequest{Method: http.MethodPost, Path: owner.Path + "/" + input.JobID + "/input", Body: body}, nil
			})
			op := result[name]
			op.Domain = owner.Domain
			result[name] = op
		}
		if owner.Cancel {
			name := "host_" + source + "_job_cancel"
			register(name, "Request cancellation from the task owner. Check its actual state afterward; cancellation is not proof of rollback.", false, managedSchema[managedJobCancel](), func(raw json.RawMessage, _ *mcpaccess.Policy) (managedRequest, error) {
				var input managedJobCancel
				_ = json.Unmarshal(raw, &input)
				if !ownerJobIDPattern.MatchString(input.JobID) {
					return managedRequest{}, errors.New("invalid_job_id")
				}
				if source == "file_archive" {
					body, _ := json.Marshal(contract.FileArchiveJobRequest{Operation: "cancel", ID: input.JobID})
					return managedRequest{Method: http.MethodPost, Path: owner.Path, Body: body}, nil
				}
				return managedRequest{Method: http.MethodPost, Path: owner.Path + "/" + input.JobID + "/cancel", Body: []byte(`{}`)}, nil
			})
			op := result[name]
			op.Domain = owner.Domain
			result[name] = op
		}
	}
}
