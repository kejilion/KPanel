package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/hostbackup"
)

// Both the script CLI and the panel use the same Agent service and writer lock.
// The command accepts structured JSON or payload bytes, never shell text/paths.
func runBackupCLI(args []string, input io.Reader, output io.Writer) error {
	if len(args) > 0 && (args[0] == "file-export" || args[0] == "file-import") {
		return runBackupFileCommand(args, input, output)
	}
	if len(args) == 0 {
		return runBackupMenu(nil, input, output)
	}
	if args[0] == "menu" {
		return runBackupMenu(args[1:], input, output)
	}
	if len(args) == 1 && args[0] == "protocol" {
		_, err := fmt.Fprintln(output, `{"protocol":1,"format":1}`)
		return err
	}
	method, path := http.MethodGet, "/v1/backups"
	binary := false
	if len(args) == 0 {
		return errors.New("backup-center: protocol | inventory | jobs | export | import | status ID | inspect ID | preview ID | restore ID | recover ID | delete ID | download ID MODULE | upload ID MODULE")
	}
	switch args[0] {
	case "inventory":
		if len(args) != 1 {
			return backup.ErrInvalid
		}
		path += "/inventory"
	case "jobs":
		if len(args) != 1 {
			return backup.ErrInvalid
		}
	case "export", "import":
		if len(args) != 1 {
			return backup.ErrInvalid
		}
		method = "POST"
	case "status", "inspect", "preview", "restore", "delete", "recover", "abort":
		if len(args) != 2 || !backup.ValidID(args[1]) {
			return backup.ErrInvalid
		}
		path += "/" + args[1]
		switch args[0] {
		case "delete":
			method = "DELETE"
		case "inspect", "preview", "recover", "abort":
			method = "POST"
			path += "/" + args[0]
		case "restore":
			method = "POST"
			path = "/v1/backups"
		}
	case "download", "upload":
		if len(args) != 3 || !backup.ValidID(args[1]) || (args[2] != "apps" && args[2] != "web" && args[2] != "docker") {
			return backup.ErrInvalid
		}
		path += "/" + args[1] + "/" + args[2]
		binary = true
		if args[0] == "upload" {
			method = "PUT"
		}
	default:
		return backup.ErrInvalid
	}
	if os.Geteuid() != 0 {
		return errors.New("backup-center requires root")
	}
	var body io.Reader
	if args[0] == "export" || args[0] == "import" || args[0] == "restore" {
		data, err := io.ReadAll(io.LimitReader(input, 16385))
		var request hostbackup.Request
		if err != nil || len(data) > 16384 || backup.Decode(data, &request) != nil {
			return backup.ErrInvalid
		}
		request.Action = args[0]
		if request.Action == "restore" {
			request.SourceID = args[1]
		}
		data, err = json.Marshal(request)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(data))
	} else if method == "PUT" {
		body = io.LimitReader(input, backup.MaxBytes+1)
	}
	token, err := backup.ReadFile(env("KEJILION_AGENT_TOKEN_FILE", "/etc/kejilion-panel/agent.token"), 4096)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, "http://unix"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	if !binary {
		req.Header.Set("Content-Type", "application/json")
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	client := http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "unix", env("KEJILION_AGENT_SOCKET", "/run/kejilion-panel/agent.sock"))
	}}}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var problem struct {
			Code string `json:"code"`
		}
		body, _ := io.ReadAll(io.LimitReader(response.Body, 16384))
		if json.Unmarshal(body, &problem) == nil && (problem.Code == "backup_host_busy" || problem.Code == "backup_busy") {
			return errors.New("请关闭宿主机终端，并等待已有主机任务完成后重试")
		}
		return fmt.Errorf("backup Agent returned HTTP %d", response.StatusCode)
	}
	limit := int64(1 << 20)
	if binary {
		limit = backup.MaxBytes
	}
	n, err := io.Copy(output, io.LimitReader(response.Body, limit+1))
	if n > limit {
		return backup.ErrInvalid
	}
	return err
}
