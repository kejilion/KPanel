package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	pathpkg "path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/ai"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/redact"
	"github.com/kejilion/kejilion-panel/internal/store"
)

type panelAITools struct{ server *Server }

var dockerMaintenanceToolBaseSchema = json.RawMessage(`{
  "type":"object",
  "required":["action"],
  "properties":{
    "action":{"type":"string"},
    "image":{"type":"string"},"target":{"type":"string"},"name":{"type":"string"},"driver":{"type":"string"},
    "containerId":{"type":"string"},"containerResourceVersion":{"type":"string"},"expectedResourceVersion":{"type":"string"},
    "confirmation":{"type":"string"},"preset":{"type":"string"},"enabled":{"type":"boolean"},"ipv6Cidr":{"type":"string"},
    "ports":{"type":"array","items":{"type":"object","required":["privatePort","publicPort"],"properties":{"privatePort":{"type":"integer"},"publicPort":{"type":"integer"},"protocol":{"type":"string"},"hostIp":{"type":"string"}},"additionalProperties":false}},
    "mounts":{"type":"array","items":{"type":"object","required":["target"],"properties":{"type":{"type":"string"},"source":{"type":"string"},"volume":{"type":"string"},"target":{"type":"string"},"readOnly":{"type":"boolean"}},"additionalProperties":false}},
    "environment":{"type":"array","items":{"type":"object","required":["name","value"],"properties":{"name":{"type":"string"},"value":{"type":"string"}},"additionalProperties":false}},
    "command":{"type":"array","items":{"type":"string"}},"network":{"type":"string"},"restartPolicy":{"type":"string"},"compose":{"type":"string"},"allowedIp":{"type":"string"},
    "backupId":{"type":"string"},"migrationHost":{"type":"string"},"migrationUser":{"type":"string"},"migrationPort":{"type":"integer"}
  },
  "additionalProperties":false
}`)

var systemActionToolSchema = json.RawMessage(`{
  "type":"object","required":["action"],
  "properties":{
    "action":{"enum":["hostname","ssh-port","ssh-defense","dns","timezone","swap","mirror","ip-preference","kernel-tuning","bbr","bbrv3","update","cleanup","reboot"]},
    "hostname":{"type":"string"},"port":{"type":"integer","minimum":1,"maximum":65535},
    "servers":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":4},
    "timezone":{"type":"string"},"swapSizeMiB":{"type":"integer","minimum":0},
    "mirrorPreset":{"enum":["cn-default","cn-edu","abroad","smart"]},
    "preference":{"enum":["ipv4","system_default"]},
    "profile":{"enum":["high","balanced","web","stream","game","off"]},
    "maintenancePolicy":{"enum":["install","update","uninstall","full","cache","standard"]},
    "confirmation":{"type":"string"},"enabled":{"type":"boolean"}
  },"additionalProperties":false
}`)

const toolReasonMaxRunes = 500

var toolReasonSchema = map[string]any{
	"type":        "string",
	"maxLength":   toolReasonMaxRunes,
	"description": "Optional model explanation for this tool call. KPanel removes it before validation, approval, audit, and host execution.",
}

func (t *panelAITools) RequiresApproval(name string, arguments json.RawMessage) bool {
	arguments, err := normalizeToolArgumentsForTool(name, arguments)
	if err != nil {
		return true
	}
	for _, definition := range t.Definitions() {
		if definition.Name == name && definition.ReadOnly {
			return false
		}
	}
	switch name {
	case "host_diagnostic_start":
		return false
	case "host_file_write":
		return false
	case "host_file_trash", "host_nginx_reload":
		return false
	case "host_system_action":
		var input struct {
			Action            string `json:"action"`
			MaintenancePolicy string `json:"maintenancePolicy"`
		}
		if json.Unmarshal(arguments, &input) != nil {
			return true
		}
		return input.Action != "cleanup" || input.MaintenancePolicy != "cache"
	case "host_app_action":
		var input struct{ Action string }
		if json.Unmarshal(arguments, &input) != nil {
			return true
		}
		switch input.Action {
		case "install", "start", "stop", "restart", "check_update", "update", "direct_access":
			return false
		default:
			return true
		}
	case "host_docker_container_action":
		var input struct{ Action string }
		if json.Unmarshal(arguments, &input) != nil {
			return true
		}
		switch input.Action {
		case "start", "stop", "restart":
			return false
		default:
			return true
		}
	case "host_site_change":
		var input struct{ Operation string }
		if json.Unmarshal(arguments, &input) != nil {
			return true
		}
		switch input.Operation {
		case "create", "update":
			return false
		default:
			return true
		}
	default:
		return true
	}
}

func (t *panelAITools) DryRun(name string, arguments json.RawMessage) error {
	arguments, err := normalizeToolArgumentsForTool(name, arguments)
	if err != nil {
		return err
	}
	for _, definition := range t.Definitions() {
		if definition.Name == name {
			if definition.ReadOnly {
				return validateReadToolArguments(name, arguments)
			}
			_, _, _, _, err = t.prepareWrite(name, arguments)
			return err
		}
	}
	return errors.New("unknown KPanel tool")
}

func (t *panelAITools) Definitions() []ai.ToolDefinition {
	readSchema := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	definitions := []ai.ToolDefinition{
		{Name: "host_system_summary", Description: "读取宿主机系统概览与 resourceVersion", Schema: readSchema, ReadOnly: true},
		{Name: "host_system_processes", Description: "采样宿主机进程 CPU 与内存排行，只返回 PID、父 PID、名称、状态、UID、CPU、内存和启动标识，不返回命令行敏感参数", Schema: readSchema, ReadOnly: true},
		{Name: "host_system_storage_usage", Description: "对固定安全根目录执行有界磁盘占用分析，返回其直接子项的递归占用排行；用于定位磁盘空间大户", Schema: json.RawMessage(`{"type":"object","required":["path"],"properties":{"path":{"enum":["/","/home","/home/docker","/home/web","/opt","/root","/srv","/tmp","/usr","/var","/var/lib/docker","/var/log"]}},"additionalProperties":false}`), ReadOnly: true},
		{Name: "host_public_network", Description: "读取宿主机公网网络信息", Schema: readSchema, ReadOnly: true},
		{Name: "host_sites_list", Description: "读取网站列表与 resourceVersion", Schema: readSchema, ReadOnly: true},
		{Name: "host_apps_list", Description: "读取应用列表、状态与 resourceVersion", Schema: readSchema, ReadOnly: true},
		{Name: "host_diagnostics_list", Description: "读取可用诊断检查", Schema: readSchema, ReadOnly: true},
		{Name: "host_docker_summary", Description: "读取 Docker 概览", Schema: readSchema, ReadOnly: true},
		{Name: "host_docker_containers", Description: "读取 Docker 容器与 resourceVersion", Schema: readSchema, ReadOnly: true},
		{Name: "host_docker_resource_usage", Description: "读取所有运行中 Docker 容器的 CPU、内存、网络、磁盘 IO 与 PID，用于资源比较和排序；这是只读查询，绝不能改用 Docker 维护任务", Schema: readSchema, ReadOnly: true},
		{Name: "host_docker_images", Description: "读取 Docker 镜像 id、标签与 resourceVersion；image_remove 前必须先调用并使用同一项的 id 和 resourceVersion", Schema: readSchema, ReadOnly: true},
		{Name: "host_docker_networks", Description: "读取 Docker 网络 id、名称与 resourceVersion；网络删除或成员变更前必须先调用", Schema: readSchema, ReadOnly: true},
		{Name: "host_docker_volumes", Description: "读取 Docker 卷名称与 resourceVersion；volume_remove 前必须先调用", Schema: readSchema, ReadOnly: true},
		{Name: "host_docker_backups", Description: "读取可恢复或迁移的 Docker 备份及 backupId；backup_restore/backup_migrate 前必须先调用", Schema: readSchema, ReadOnly: true},
		{Name: "host_docker_environment", Description: "读取当前 Docker daemon 镜像源和 IPv6 配置；daemon_mirror/daemon_ipv6 前必须先调用", Schema: readSchema, ReadOnly: true},
		{Name: "host_docker_jobs", Description: "读取 Docker 后台任务", Schema: readSchema, ReadOnly: true},
		{Name: "host_system_action", Description: "执行 KPanel 已注册的结构化系统操作；磁盘清理使用 action=cleanup，maintenancePolicy=cache 或 standard，先读取磁盘占用再执行", Schema: systemActionToolSchema},
		{Name: "host_app_action", Description: "安装、启停、更新或卸载已注册应用", Schema: json.RawMessage(`{"type":"object","required":["appId","action"],"properties":{"appId":{"type":"string"},"action":{"enum":["install","start","stop","restart","check_update","update","uninstall","direct_access"]},"hostPort":{"type":"integer"},"accessMode":{"type":"string"},"resourceVersion":{"type":"string"}},"additionalProperties":false}`)},
		{Name: "host_docker_container_action", Description: "启停、重启或删除容器", Schema: json.RawMessage(`{"type":"object","required":["containerId","action","resourceVersion"],"properties":{"containerId":{"type":"string"},"action":{"enum":["start","stop","restart","remove"]},"resourceVersion":{"type":"string"}},"additionalProperties":false}`)},
		{Name: "host_diagnostic_start", Description: "启动固定诊断检查并返回任务 ID", Schema: json.RawMessage(`{"type":"object","required":["checkId"],"properties":{"checkId":{"type":"string"}},"additionalProperties":false}`)},
		{Name: "host_docker_task", Description: "仅在用户明确要求修改 Docker 配置、创建/删除资源、清理或备份时启动结构化维护任务；根据 action、真实状态和用户意图自主选择相关字段，KPanel 只保留鉴权、审批、固定动作路由和审计边界；涉及现有资源或 daemon 配置时先调用对应只读工具；状态和资源占用查询禁止使用此工具", Schema: dockerMaintenanceToolSchema},
		{Name: "host_file_list", Description: "列出受 KPanel File Manager 保护规则约束的目录，返回文件 resourceVersion", Schema: json.RawMessage(`{"type":"object","required":["path"],"properties":{"path":{"type":"string","maxLength":4096},"limit":{"type":"integer","minimum":1,"maximum":200},"search":{"type":"string","maxLength":128}},"additionalProperties":false}`), ReadOnly: true},
		{Name: "host_file_read", Description: "读取受 KPanel File Manager 保护规则约束的 UTF-8 文本文件，最大 64 KiB", Schema: json.RawMessage(`{"type":"object","required":["path"],"properties":{"path":{"type":"string","maxLength":4096}},"additionalProperties":false}`), ReadOnly: true},
		{Name: "host_file_tail", Description: "读取受保护规则约束的大型 UTF-8 日志文件尾部，适合查看 Nginx 与服务错误日志", Schema: json.RawMessage(`{"type":"object","required":["path"],"properties":{"path":{"type":"string","maxLength":4096},"maxBytes":{"type":"integer","minimum":1024,"maximum":65536}},"additionalProperties":false}`), ReadOnly: true},
		{Name: "host_file_write", Description: "使用 resourceVersion 覆盖现有 UTF-8 文本文件，最大 64 KiB；受保护和只读目录永远不可写", Schema: json.RawMessage(`{"type":"object","required":["path","content","expectedResourceVersion"],"properties":{"path":{"type":"string","maxLength":4096},"content":{"type":"string","maxLength":65536},"expectedResourceVersion":{"type":"string"}},"additionalProperties":false}`)},
		{Name: "host_file_trash", Description: "把最多 20 个已读取且版本未变化的文件或目录移入 KPanel 回收站；不执行永久删除，核心路径仍不可触碰", Schema: json.RawMessage(`{"type":"object","required":["sources","expectedResourceVersions"],"properties":{"sources":{"type":"array","items":{"type":"string","maxLength":4096},"minItems":1,"maxItems":20},"expectedResourceVersions":{"type":"object","additionalProperties":{"type":"string"}}},"additionalProperties":false}`)},
		{Name: "host_nginx_test", Description: "对 KPanel 管理的 Nginx 容器执行固定 nginx -t，返回语法错误位置；修改配置后必须先调用", Schema: readSchema, ReadOnly: true},
		{Name: "host_nginx_reload", Description: "先执行固定 nginx -t，通过后才安全 reload KPanel 管理的 Nginx；配置无效时拒绝重载", Schema: readSchema},
		{Name: "host_site_change", Description: "创建、更新或删除网站配置", Schema: json.RawMessage(`{"type":"object","required":["operation"],"properties":{"operation":{"enum":["create","update","delete"]},"siteId":{"type":"string"},"payload":{"type":"object"}},"additionalProperties":false}`)},
		{Name: "host_job_input", Description: "向已存在的 KPanel 交互任务提交输入", Schema: json.RawMessage(`{"type":"object","required":["kind","jobId","data"],"properties":{"kind":{"enum":["site","app","diagnostic"]},"jobId":{"type":"string"},"data":{"type":"string","maxLength":16384}},"additionalProperties":false}`)},
		{Name: "host_docker_exec", Description: "在指定 Docker 容器内执行受 Agent 校验的命令", Schema: json.RawMessage(`{"type":"object","required":["containerId","resourceVersion","command"],"properties":{"containerId":{"type":"string"},"resourceVersion":{"type":"string"},"command":{"type":"array","items":{"type":"string"},"maxItems":32}},"additionalProperties":false}`)},
	}
	for index := range definitions {
		definitions[index].Schema = withToolReasonMetadata(definitions[index].Schema)
	}
	return definitions
}

func (t *panelAITools) Execute(ctx context.Context, execution ai.ToolExecutionContext, name string, arguments json.RawMessage) (string, error) {
	if t == nil || t.server == nil {
		return "", errors.New("panel tool service is unavailable")
	}
	var err error
	arguments, err = normalizeToolArgumentsForTool(name, arguments)
	if err != nil {
		return "", toolArgumentError(err)
	}
	known, readOnly := false, false
	for _, definition := range t.Definitions() {
		if definition.Name == name {
			known, readOnly = true, definition.ReadOnly
			break
		}
	}
	if !known {
		return "", errors.New("unknown KPanel tool")
	}
	if readOnly {
		if err := validateReadToolArguments(name, arguments); err != nil {
			return "", toolArgumentError(err)
		}
	}
	readPaths := map[string]string{
		"host_system_summary": "/v1/system/summary", "host_public_network": "/v1/system/public-network", "host_sites_list": "/v1/sites",
		"host_system_processes": "/v1/system/processes", "host_nginx_test": "/v1/nginx/test",
		"host_apps_list": "/v1/apps", "host_diagnostics_list": "/v1/diagnostics", "host_docker_summary": "/v1/docker/summary",
		"host_docker_containers": "/v1/docker/containers", "host_docker_images": "/v1/docker/images", "host_docker_networks": "/v1/docker/networks",
		"host_docker_volumes": "/v1/docker/volumes", "host_docker_backups": "/v1/docker/backups", "host_docker_environment": "/v1/docker/environment",
		"host_docker_jobs": "/v1/docker/jobs", "host_docker_resource_usage": "/v1/docker/container-stats",
	}
	requestID := newRequestID()
	if name == "host_system_storage_usage" {
		var input struct {
			Path string `json:"path"`
		}
		if err := decodeStrictToolArguments(arguments, &input); err != nil || !aiStorageRoot(input.Path) {
			return "", errors.New("invalid storage usage query")
		}
		response, err := t.server.hostOps.Get(ctx, "/v1/system/storage-usage", url.Values{"path": []string{input.Path}}.Encode(), requestID)
		return t.finish(execution, name, input.Path, requestID, nil, response, err)
	}
	if name == "host_file_list" || name == "host_file_read" || name == "host_file_tail" {
		var input struct {
			Path     string `json:"path"`
			Limit    int    `json:"limit,omitempty"`
			Search   string `json:"search,omitempty"`
			MaxBytes int    `json:"maxBytes,omitempty"`
		}
		if err := decodeStrictToolArguments(arguments, &input); err != nil || input.Path == "" || len(input.Path) > 4096 || len(input.Search) > 128 {
			return "", errors.New("invalid file query")
		}
		if name != "host_file_list" && !aiFileReadable(input.Path) {
			return "", errors.New("AI access to sensitive file content is forbidden")
		}
		values := url.Values{"path": []string{input.Path}}
		path := "/v1/files/text"
		if name == "host_file_list" {
			path = "/v1/files"
			if input.Limit == 0 {
				input.Limit = 100
			}
			if input.Limit < 1 || input.Limit > 200 {
				return "", errors.New("invalid file list limit")
			}
			values.Set("limit", fmt.Sprint(input.Limit))
			if input.Search != "" {
				values.Set("search", input.Search)
			}
		} else if name == "host_file_tail" {
			path = "/v1/files/tail"
			if input.MaxBytes == 0 {
				input.MaxBytes = 32 << 10
			}
			if input.MaxBytes < 1024 || input.MaxBytes > 64<<10 {
				return "", errors.New("invalid file tail limit")
			}
			values.Set("maxBytes", fmt.Sprint(input.MaxBytes))
		}
		response, err := t.server.hostOps.Get(ctx, path, values.Encode(), requestID)
		return t.finish(execution, name, input.Path, requestID, nil, response, err)
	}
	if path := readPaths[name]; path != "" {
		if err := decodeStrictToolArguments(arguments, &struct{}{}); err != nil {
			return "", errors.New("invalid read tool arguments")
		}
		response, err := t.server.hostOps.Get(ctx, path, "", requestID)
		return t.finish(execution, name, path, requestID, nil, response, err)
	}
	method, path, target, body, err := t.prepareWrite(name, arguments)
	if err != nil {
		return "", toolArgumentError(err)
	}
	change := map[string]any{"tool": name, "target": target, "sessionId": execution.SessionID, "runId": execution.RunID, "toolCallId": execution.ToolCallID, "arguments": safeArgumentSummary(arguments)}
	if err := t.server.store.AppendAudit(store.AuditEvent{ID: newRequestID(), OccurredAt: time.Now().UTC(), ActorType: "user", ActorID: execution.UserID, Action: "ai.tool." + name, TargetKind: "host_operation", TargetID: target, Result: "intent", RequestID: requestID, Change: change}, store.MaxAuditEntries); err != nil {
		return "", errors.New("AI tool audit unavailable")
	}
	rawQuery := ""
	if name == "host_file_write" {
		rawQuery = url.Values{"path": []string{target}}.Encode()
	}
	response, callErr := t.server.hostOps.Do(ctx, method, path, rawQuery, requestID, body)
	return t.finish(execution, name, target, requestID, change, response, callErr)
}

func (t *panelAITools) prepareWrite(name string, raw json.RawMessage) (method, path, target string, body []byte, err error) {
	switch name {
	case "host_system_action":
		var input contract.SystemActionRequest
		if err = decodeStrictToolArguments(raw, &input); err != nil {
			return
		}
		if field, detail := validateSystemAction(&input); field != "" {
			err = fmt.Errorf("%s: %s", field, detail)
			return
		}
		method, path, target = http.MethodPost, "/v1/system/actions", input.Action
		body, err = json.Marshal(input)
	case "host_app_action":
		var input struct {
			AppID, Action               string
			HostPort                    *int `json:"hostPort"`
			AccessMode, ResourceVersion *string
		}
		if err = decodeStrictToolArguments(raw, &input); err != nil {
			return
		}
		if !appIDPattern.MatchString(input.AppID) {
			err = errors.New("invalid appId")
			return
		}
		public := "/api/v1/apps/" + input.AppID + "/" + input.Action
		var ok bool
		path, _, _, ok = allowedAppActionPath(public)
		if !ok {
			err = errors.New("unsupported app action")
			return
		}
		payload := map[string]any{}
		if input.HostPort != nil {
			payload["hostPort"] = *input.HostPort
		}
		if input.AccessMode != nil {
			payload["accessMode"] = *input.AccessMode
		}
		if input.ResourceVersion != nil {
			payload["resourceVersion"] = *input.ResourceVersion
		}
		method, target = http.MethodPost, input.AppID
		body, err = json.Marshal(payload)
	case "host_docker_container_action":
		var input struct{ ContainerID, Action, ResourceVersion string }
		if err = decodeStrictToolArguments(raw, &input); err != nil {
			return
		}
		public := "/api/v1/docker/containers/" + input.ContainerID + "/" + input.Action
		var ok bool
		path, target, _, ok = allowedDockerActionPath(public)
		if !ok || !resourceVersionPattern.MatchString(input.ResourceVersion) {
			err = errors.New("invalid container action or resourceVersion")
			return
		}
		method = http.MethodPost
		body, err = json.Marshal(map[string]string{"resourceVersion": input.ResourceVersion})
	case "host_diagnostic_start":
		var input struct {
			CheckID string `json:"checkId"`
		}
		if err = decodeStrictToolArguments(raw, &input); err != nil {
			return
		}
		if !diagnosticCheckIDPattern.MatchString(input.CheckID) {
			err = errors.New("invalid checkId")
			return
		}
		method, path, target, body = http.MethodPost, "/v1/diagnostic-jobs", input.CheckID, raw
	case "host_docker_task":
		var input dockerx.MaintenanceInput
		if err = decodeStrictToolArguments(raw, &input); err != nil {
			return
		}
		input.Action = strings.TrimSpace(input.Action)
		if !dockerx.IsMaintenanceAction(input.Action) {
			err = errors.New("unsupported Docker maintenance action")
			return
		}
		method, path, target = http.MethodPost, "/v1/docker/tasks", input.Action
		body, err = json.Marshal(input)
	case "host_file_write":
		var input struct {
			Path                    string `json:"path"`
			Content                 string `json:"content"`
			ExpectedResourceVersion string `json:"expectedResourceVersion"`
		}
		if err = decodeStrictToolArguments(raw, &input); err != nil {
			return
		}
		if input.Path == "" || len(input.Path) > 4096 || len(input.Content) > 64<<10 || !resourceVersionPattern.MatchString(input.ExpectedResourceVersion) || !aiFileMutable(input.Path) {
			err = errors.New("invalid file write request")
			return
		}
		method, path, target = http.MethodPut, "/v1/files/content", input.Path
		body, err = json.Marshal(contract.FileWriteRequest{Content: input.Content, ExpectedResourceVersion: input.ExpectedResourceVersion})
	case "host_file_trash":
		var input struct {
			Sources                  []string          `json:"sources"`
			ExpectedResourceVersions map[string]string `json:"expectedResourceVersions"`
		}
		if err = decodeStrictToolArguments(raw, &input); err != nil {
			return
		}
		if len(input.Sources) < 1 || len(input.Sources) > 20 || len(input.ExpectedResourceVersions) != len(input.Sources) {
			err = errors.New("invalid file trash request")
			return
		}
		for _, source := range input.Sources {
			if source == "" || len(source) > 4096 || !resourceVersionPattern.MatchString(input.ExpectedResourceVersions[source]) || !aiFileMutable(source) {
				err = errors.New("invalid file trash source or resourceVersion")
				return
			}
		}
		method, path, target = http.MethodPost, "/v1/files/actions", input.Sources[0]
		body, err = json.Marshal(contract.FileActionRequest{Action: "trash", Sources: input.Sources, ExpectedResourceVersions: input.ExpectedResourceVersions})
	case "host_nginx_reload":
		if err = decodeStrictToolArguments(raw, &struct{}{}); err != nil {
			return
		}
		method, path, target, body = http.MethodPost, "/v1/nginx/reload", "nginx", []byte(`{}`)
	case "host_site_change":
		var input struct {
			Operation, SiteID string
			Payload           json.RawMessage
		}
		if err = decodeStrictToolArguments(raw, &input); err != nil {
			return
		}
		switch input.Operation {
		case "create":
			method, path, target = http.MethodPost, "/v1/sites", "new-site"
		case "update":
			if !siteIDPattern.MatchString(input.SiteID) {
				err = errors.New("invalid siteId")
				return
			}
			method, path, target = http.MethodPatch, "/v1/sites/"+input.SiteID, input.SiteID
		case "delete":
			if !siteIDPattern.MatchString(input.SiteID) {
				err = errors.New("invalid siteId")
				return
			}
			method, path, target = http.MethodDelete, "/v1/sites/"+input.SiteID, input.SiteID
		default:
			err = errors.New("unsupported site operation")
			return
		}
		body = input.Payload
	case "host_job_input":
		var input struct{ Kind, JobID, Data string }
		if err = decodeStrictToolArguments(raw, &input); err != nil {
			return
		}
		if !siteIDPattern.MatchString(input.JobID) || input.Data == "" || len(input.Data) > 16<<10 || strings.IndexByte(input.Data, 0) >= 0 {
			err = errors.New("invalid job input")
			return
		}
		prefixes := map[string]string{"site": "/v1/site-installations/", "app": "/v1/app-jobs/", "diagnostic": "/v1/diagnostic-jobs/"}
		prefix := prefixes[input.Kind]
		if prefix == "" {
			err = errors.New("invalid job kind")
			return
		}
		method, path, target = http.MethodPost, prefix+input.JobID+"/input", input.JobID
		body, err = json.Marshal(map[string]string{"data": input.Data})
	case "host_docker_exec":
		var input struct {
			ContainerID     string   `json:"containerId"`
			ResourceVersion string   `json:"resourceVersion"`
			Command         []string `json:"command"`
		}
		if err = decodeStrictToolArguments(raw, &input); err != nil {
			return
		}
		if !containerIDPattern.MatchString(input.ContainerID) || !resourceVersionPattern.MatchString(input.ResourceVersion) || len(input.Command) == 0 || len(input.Command) > 32 {
			err = errors.New("invalid Docker exec request")
			return
		}
		method, path, target = http.MethodPost, "/v1/docker/containers/"+input.ContainerID+"/exec", input.ContainerID
		body, err = json.Marshal(map[string]any{"resourceVersion": input.ResourceVersion, "command": input.Command})
	default:
		err = errors.New("unknown KPanel tool")
	}
	return
}

func (t *panelAITools) finish(execution ai.ToolExecutionContext, name, target, requestID string, change map[string]any, response AgentResponse, callErr error) (string, error) {
	result := "success"
	if callErr != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		result = "failure"
	}
	if change == nil {
		change = map[string]any{"tool": name, "target": target, "sessionId": execution.SessionID, "runId": execution.RunID, "toolCallId": execution.ToolCallID}
	}
	_ = t.server.store.AppendAudit(store.AuditEvent{ID: newRequestID(), OccurredAt: time.Now().UTC(), ActorType: "user", ActorID: execution.UserID, Action: "ai.tool." + name, TargetKind: "host_operation", TargetID: target, Result: result, RequestID: requestID, Change: change}, store.MaxAuditEntries)
	if callErr != nil {
		return "", errors.New("Agent unavailable")
	}
	if result == "failure" {
		if response.StatusCode == http.StatusConflict || response.StatusCode == http.StatusPreconditionFailed {
			return "", fmt.Errorf("%w: Agent returned HTTP %d", ai.ErrToolConflict, response.StatusCode)
		}
		var problem contract.Problem
		hasProblem := json.Unmarshal(response.Body, &problem) == nil && problem.Code != ""
		if hasProblem {
			if classified := classifyAgentToolProblem(name, problem); classified != nil {
				return "", classified
			}
		}
		if hasProblem && recoverableAgentToolStatus(response.StatusCode) {
			return "", &ai.ToolRejectedError{StatusCode: response.StatusCode, Code: problem.Code, RequestID: problem.RequestID}
		}
		if hasProblem {
			if problem.RequestID != "" {
				return "", fmt.Errorf("Agent request failed (HTTP %d, %s, requestId: %s)", response.StatusCode, problem.Code, problem.RequestID)
			}
			return "", fmt.Errorf("Agent request failed (HTTP %d, %s)", response.StatusCode, problem.Code)
		}
		return "", fmt.Errorf("Agent request failed (HTTP %d)", response.StatusCode)
	}
	return string(response.Body), nil
}

func classifyAgentToolProblem(name string, problem contract.Problem) error {
	if name != "host_docker_task" {
		return nil
	}
	switch problem.Code {
	case "docker_task_invalid", "invalid_request":
		return toolArgumentError(errors.New("Agent rejected the Docker action arguments; re-check the requested action, current resource state and user-provided values before retrying"))
	case "docker_resource_not_found":
		return fmt.Errorf("%w: Docker resource no longer exists; re-read current Docker state", ai.ErrToolConflict)
	default:
		return nil
	}
}

func recoverableAgentToolStatus(status int) bool {
	if status < 400 || status >= 500 {
		return false
	}
	switch status {
	case http.StatusUnauthorized, http.StatusRequestTimeout, http.StatusConflict, http.StatusPreconditionFailed, http.StatusTooManyRequests:
		return false
	default:
		return true
	}
}

func withToolReasonMetadata(schema json.RawMessage) json.RawMessage {
	var document map[string]any
	if err := json.Unmarshal(schema, &document); err != nil || document["type"] != "object" {
		return schema
	}
	properties, ok := document["properties"].(map[string]any)
	if !ok {
		properties = map[string]any{}
		document["properties"] = properties
	}
	properties["reason"] = toolReasonSchema
	updated, err := json.Marshal(document)
	if err != nil {
		return schema
	}
	return updated
}

func toolArgumentError(err error) error {
	return fmt.Errorf("%w: %v", ai.ErrToolArguments, err)
}

func normalizeToolArguments(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var arguments map[string]json.RawMessage
	if err := decoder.Decode(&arguments); err != nil || arguments == nil {
		return nil, errors.New("invalid tool arguments")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("invalid tool arguments")
	}
	if encodedReason, ok := arguments["reason"]; ok {
		var reason string
		if err := json.Unmarshal(encodedReason, &reason); err != nil || utf8.RuneCountInString(reason) > toolReasonMaxRunes {
			return nil, errors.New("invalid tool reason metadata")
		}
		delete(arguments, "reason")
	}
	normalized, err := json.Marshal(arguments)
	if err != nil {
		return nil, errors.New("invalid tool arguments")
	}
	return normalized, nil
}

func normalizeToolArgumentsForTool(name string, raw json.RawMessage) (json.RawMessage, error) {
	normalized, err := normalizeToolArguments(raw)
	if err != nil || !zeroArgumentReadTool(name) {
		return normalized, err
	}
	var arguments map[string]json.RawMessage
	if json.Unmarshal(normalized, &arguments) != nil {
		return normalized, nil
	}
	placeholder, ok := arguments["_"]
	if !ok {
		return normalized, nil
	}
	var ignored bool
	if json.Unmarshal(placeholder, &ignored) != nil {
		return normalized, nil
	}
	delete(arguments, "_")
	return json.Marshal(arguments)
}

func zeroArgumentReadTool(name string) bool {
	switch name {
	case "host_system_summary", "host_system_processes", "host_public_network", "host_sites_list",
		"host_apps_list", "host_diagnostics_list", "host_docker_summary", "host_docker_containers",
		"host_docker_resource_usage", "host_docker_images", "host_docker_networks", "host_docker_volumes",
		"host_docker_backups", "host_docker_environment", "host_docker_jobs", "host_nginx_test":
		return true
	default:
		return false
	}
}

func validateReadToolArguments(name string, raw json.RawMessage) error {
	switch name {
	case "host_system_storage_usage":
		var input struct {
			Path string `json:"path"`
		}
		if err := decodeStrictToolArguments(raw, &input); err != nil || !aiStorageRoot(input.Path) {
			return errors.New("invalid storage usage query")
		}
	case "host_file_list", "host_file_read", "host_file_tail":
		var input struct {
			Path     string `json:"path"`
			Limit    int    `json:"limit,omitempty"`
			Search   string `json:"search,omitempty"`
			MaxBytes int    `json:"maxBytes,omitempty"`
		}
		if err := decodeStrictToolArguments(raw, &input); err != nil || input.Path == "" || len(input.Path) > 4096 || len(input.Search) > 128 {
			return errors.New("invalid file query")
		}
		if name == "host_file_list" && (input.Limit < 0 || input.Limit > 200) {
			return errors.New("invalid file list limit")
		}
		if name == "host_file_tail" && (input.MaxBytes != 0 && (input.MaxBytes < 1024 || input.MaxBytes > 64<<10)) {
			return errors.New("invalid file tail limit")
		}
		if name != "host_file_list" && !aiFileReadable(input.Path) {
			return errors.New("AI access to sensitive file content is forbidden")
		}
	default:
		if err := decodeStrictToolArguments(raw, &struct{}{}); err != nil {
			return errors.New("invalid read tool arguments")
		}
	}
	return nil
}

// topSecretSummaries are wholesale-replaced at any nesting depth, matching the
// previous top-level behavior for these keys.
var topSecretSummaries = map[string]struct{}{
	"data": {}, "content": {}, "password": {}, "token": {}, "apiKey": {}, "key": {}, "command": {}, "reason": {},
}

func safeArgumentSummary(raw json.RawMessage) map[string]any {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return map[string]any{"valid": false}
	}
	return redactSummaryValue(value).(map[string]any)
}

// redactSummaryValue walks nested tool arguments so that values which never
// sit at the top level (compose blobs, environment arrays) still pass through
// the central redactor instead of landing in the audit store verbatim.
func redactSummaryValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		summary := make(map[string]any, len(typed))
		for key, item := range typed {
			if _, drop := topSecretSummaries[key]; drop {
				summary[key] = "[REDACTED]"
				continue
			}
			summary[key] = redactSummaryValue(item)
		}
		return summary
	case []any:
		list := make([]any, len(typed))
		for index, item := range typed {
			list[index] = redactSummaryValue(item)
		}
		return list
	case string:
		return redact.Text(typed)
	default:
		return value
	}
}

func aiFileReadable(raw string) bool {
	clean, ok := normalizedAIFilePath(raw)
	if !ok || hasPathPrefix(clean, "/proc") || hasPathPrefix(clean, "/sys") || hasPathPrefix(clean, "/dev") ||
		hasPathPrefix(clean, "/etc/ssl/private") || hasPathPrefix(clean, "/etc/letsencrypt") ||
		hasPathPrefix(clean, "/etc/wireguard") || hasPathPrefix(clean, "/var/lib/docker/swarm") ||
		hasPathPrefix(clean, "/home/docker/kpanel") || strings.Contains(clean, "/.ssh/") || strings.HasSuffix(clean, "/.ssh") {
		return false
	}
	base := strings.ToLower(pathpkg.Base(clean))
	if base == "shadow" || base == "gshadow" || base == "credentials" || base == "credentials.json" ||
		base == "auth.json" || base == "agent.token" || base == "token" || base == "secrets" ||
		base == "id_rsa" || base == "id_ed25519" || base == "id_ecdsa" || base == "id_dsa" ||
		strings.HasPrefix(base, ".env") || strings.HasSuffix(base, ".key") || strings.HasSuffix(base, ".pem") ||
		strings.HasSuffix(base, ".p12") || strings.HasSuffix(base, ".pfx") || base == "environ" {
		return false
	}
	return true
}

func aiFileMutable(raw string) bool {
	clean, ok := normalizedAIFilePath(raw)
	if !ok || strings.Contains(clean, "/.ssh/") || strings.HasSuffix(clean, "/.ssh") {
		return false
	}
	for _, prefix := range []string{
		"/proc", "/sys", "/dev", "/run", "/boot", "/usr", "/bin", "/sbin", "/lib", "/lib64",
		"/var/lib/kejilion-panel", "/var/lib/docker", "/home/docker/kpanel", "/etc/kejilion-panel",
		"/etc/systemd", "/etc/sudoers.d", "/etc/pam.d", "/etc/security", "/etc/ssh", "/var/spool/cron",
	} {
		if hasPathPrefix(clean, prefix) {
			return false
		}
	}
	switch clean {
	case "/etc/passwd", "/etc/shadow", "/etc/group", "/etc/gshadow", "/etc/sudoers", "/etc/fstab", "/etc/crontab":
		return false
	}
	return true
}

func normalizedAIFilePath(raw string) (string, bool) {
	if raw == "" || len(raw) > 4096 || strings.IndexByte(raw, 0) >= 0 || !strings.HasPrefix(raw, "/") {
		return "", false
	}
	clean := pathpkg.Clean(raw)
	return clean, clean != "/"
}

func hasPathPrefix(value, prefix string) bool {
	return value == prefix || strings.HasPrefix(value, strings.TrimSuffix(prefix, "/")+"/")
}

func aiStorageRoot(value string) bool {
	switch value {
	case "/", "/home", "/home/docker", "/home/web", "/opt", "/root", "/srv", "/tmp", "/usr", "/var", "/var/lib/docker", "/var/log":
		return true
	default:
		return false
	}
}

func decodeStrictToolArguments(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("tool arguments must contain exactly one JSON object")
	}
	return nil
}
