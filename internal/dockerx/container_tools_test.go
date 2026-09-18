package dockerx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContainerStatsReturnsBoundedSingleSample(t *testing.T) {
	id := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/containers/" + id + "/json":
			_ = json.NewEncoder(response).Encode(managedInspect(id, "2026-01-01T00:00:00Z", 0))
		case "/containers/" + id + "/stats":
			if request.URL.Query().Get("stream") != "false" || request.URL.Query().Get("one-shot") != "false" {
				t.Fatalf("unexpected stats query: %s", request.URL.RawQuery)
			}
			_, _ = response.Write([]byte(`{
				"cpu_stats":{"cpu_usage":{"total_usage":300,"percpu_usage":[150,150]},"system_cpu_usage":3000,"online_cpus":2},
				"precpu_stats":{"cpu_usage":{"total_usage":200},"system_cpu_usage":2000},
				"memory_stats":{"usage":1000,"limit":2000,"stats":{"inactive_file":100}},
				"networks":{"eth0":{"rx_bytes":11,"tx_bytes":13}},
				"blkio_stats":{"io_service_bytes_recursive":[{"op":"Read","value":17},{"op":"Write","value":19}]},
				"pids_stats":{"current":7}
			}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := testHTTPClient(server)
	stats, err := client.ContainerStats(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CPUPercent != 20 || !stats.CPUCountersAvailable ||
		stats.CPUTotalUsage != 300 || stats.SystemCPUUsage != 3000 || stats.CPUOnlineCPUs != 2 ||
		stats.MemoryBytes != 900 || stats.MemoryPercent != 45 ||
		stats.NetworkRx != 11 || stats.NetworkTx != 13 ||
		stats.BlockRead != 17 || stats.BlockWrite != 19 || stats.PIDs != 7 {
		t.Fatalf("unexpected container stats: %#v", stats)
	}
	encoded, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	for _, internalField := range []string{"CPUTotalUsage", "SystemCPUUsage", "CPUOnlineCPUs", "CPUCountersAvailable"} {
		if strings.Contains(string(encoded), internalField) {
			t.Fatalf("internal CPU counter %q leaked into API response: %s", internalField, encoded)
		}
	}
}

func TestContainerStatsDoesNotUnderflowAfterCounterReset(t *testing.T) {
	id := strings.Repeat("9", 64)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/containers/" + id + "/json":
			_ = json.NewEncoder(response).Encode(managedInspect(id, "2026-01-01T00:00:00Z", 0))
		case "/containers/" + id + "/stats":
			_, _ = response.Write([]byte(`{
				"cpu_stats":{"cpu_usage":{"total_usage":10},"system_cpu_usage":100,"online_cpus":2},
				"precpu_stats":{"cpu_usage":{"total_usage":20},"system_cpu_usage":200}
			}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	client := testHTTPClient(server)
	stats, err := client.ContainerStats(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CPUPercent != 0 {
		t.Fatalf("CPU after counter reset = %f, want 0", stats.CPUPercent)
	}
}

func TestRunningContainerStatsUsesOneListWithoutInspectAndBoundsWork(t *testing.T) {
	firstID := strings.Repeat("1", 64)
	secondID := strings.Repeat("2", 64)
	thirdID := strings.Repeat("3", 64)
	listCalls := 0
	statsCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/containers/json":
			listCalls++
			if request.URL.Query().Get("all") != "0" || request.URL.Query().Get("size") != "0" {
				t.Fatalf("unexpected list query: %s", request.URL.RawQuery)
			}
			_, _ = response.Write([]byte(`[
				{"Id":"` + secondID + `","Names":["/bravo"],"Image":"b:latest","State":"running"},
				{"Id":"` + firstID + `","Names":["/alpha"],"Image":"a:latest","State":"running"},
				{"Id":"` + thirdID + `","Names":["/stopped"],"Image":"c:latest","State":"exited"}
			]`))
		case strings.HasSuffix(request.URL.Path, "/stats"):
			statsCalls++
			if strings.HasSuffix(request.URL.Path, "/json") {
				t.Fatal("bulk monitoring performed a container inspect")
			}
			if request.URL.Query().Get("stream") != "false" || request.URL.Query().Get("one-shot") != "true" {
				t.Fatalf("unexpected bulk stats query: %s", request.URL.RawQuery)
			}
			_, _ = response.Write([]byte(`{
				"cpu_stats":{"cpu_usage":{"total_usage":300},"system_cpu_usage":3000,"online_cpus":2},
				"precpu_stats":{"cpu_usage":{"total_usage":200},"system_cpu_usage":2000},
				"memory_stats":{"usage":1000,"limit":2000}
			}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	batch, err := testHTTPClient(server).RunningContainerStats(context.Background(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if listCalls != 1 || statsCalls != 1 || batch.Total != 2 || batch.Truncated != 1 ||
		len(batch.Items) != 1 || batch.Items[0].Name != "alpha" {
		t.Fatalf("unexpected bulk stats: %#v list=%d stats=%d", batch, listCalls, statsCalls)
	}
}

func TestContainerExecUsesFixedShellAndDoesNotReturnCommand(t *testing.T) {
	id := strings.Repeat("b", 64)
	execID := strings.Repeat("c", 64)
	command := "printf hello"
	var createPayload struct {
		Cmd []string `json:"Cmd"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/containers/"+id+"/json":
			_ = json.NewEncoder(response).Encode(managedInspect(id, "2026-01-01T00:00:00Z", 0))
		case request.Method == http.MethodPost && request.URL.Path == "/containers/"+id+"/exec":
			if err := json.NewDecoder(request.Body).Decode(&createPayload); err != nil {
				t.Fatal(err)
			}
			_, _ = response.Write([]byte(`{"Id":"` + execID + `"}`))
		case request.Method == http.MethodPost && request.URL.Path == "/exec/"+execID+"/start":
			_, _ = response.Write([]byte("hello\n"))
		case request.Method == http.MethodGet && request.URL.Path == "/exec/"+execID+"/json":
			_, _ = response.Write([]byte(`{"ID":"` + execID + `","Running":false,"ExitCode":0}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := testHTTPClient(server)
	inspect := managedInspect(id, "2026-01-01T00:00:00Z", 0)
	version := client.summaryFromInspect(inspect).ResourceVersion
	result, err := client.ContainerExec(context.Background(), id, ContainerExecInput{
		ResourceVersion: version,
		Command:         command,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(createPayload.Cmd, "\x00") != "/bin/sh\x00-lc\x00"+command {
		t.Fatalf("exec command = %#v", createPayload.Cmd)
	}
	if result.Output != "hello" || result.ExitCode != 0 ||
		strings.Contains(result.Output, command) {
		t.Fatalf("unexpected exec result: %#v", result)
	}
}

func TestContainerExecRejectsMultilineCommandBeforeDocker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("invalid command reached Docker")
	}))
	defer server.Close()
	client := testHTTPClient(server)
	_, err := client.ContainerExec(
		context.Background(),
		strings.Repeat("a", 64),
		ContainerExecInput{ResourceVersion: "sha256:" + strings.Repeat("0", 64), Command: "id\nuname"},
	)
	if err != ErrActionUnsupported {
		t.Fatalf("invalid command error = %v", err)
	}
}

func TestManagedContainerCreateIsStructuredAndRollsBackStartFailure(t *testing.T) {
	createdID := strings.Repeat("d", 64)
	var payload struct {
		Image        string            `json:"Image"`
		Env          []string          `json:"Env"`
		Labels       map[string]string `json:"Labels"`
		ExposedPorts map[string]any    `json:"ExposedPorts"`
		HostConfig   struct {
			PortBindings map[string][]map[string]string `json:"PortBindings"`
			Mounts       []struct {
				Type, Source, Target string
				ReadOnly             bool
			} `json:"Mounts"`
			RestartPolicy struct {
				Name string `json:"Name"`
			} `json:"RestartPolicy"`
		} `json:"HostConfig"`
	}
	var rollback bool
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/containers/create":
			if request.URL.Query().Get("name") != "demo" {
				t.Fatalf("create name = %q", request.URL.Query().Get("name"))
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			_, _ = response.Write([]byte(`{"Id":"` + createdID + `"}`))
		case request.Method == http.MethodPost && request.URL.Path == "/containers/"+createdID+"/start":
			response.WriteHeader(http.StatusInternalServerError)
			_, _ = response.Write([]byte(`{"message":"start failed"}`))
		case request.Method == http.MethodDelete && request.URL.Path == "/containers/"+createdID:
			rollback = request.URL.Query().Get("force") == "1"
			response.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := testHTTPClient(server)
	err := client.createManagedContainer(context.Background(), MaintenanceInput{
		Action: "container_create", Name: "demo", Image: "nginx:alpine",
		RestartPolicy: "unless-stopped",
		Ports:         []ContainerCreatePort{{PrivatePort: 80, PublicPort: 8080, Protocol: "tcp", HostIP: "192.0.2.10"}},
		Mounts: []ContainerCreateMount{
			{Type: "bind", Source: "/home/docker/demo", Target: "/data"},
			{Type: "volume", Source: "demo-cache", Target: "/cache", ReadOnly: true},
		},
		Environment: []ContainerCreateEnvironment{{Name: "APP_MODE", Value: "production"}},
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") || !rollback {
		t.Fatalf("create rollback result: err=%v rollback=%v", err, rollback)
	}
	if payload.Image != "nginx:alpine" ||
		payload.Labels["io.kejilion.panel.managed"] != "true" ||
		len(payload.Env) != 1 || payload.Env[0] != "APP_MODE=production" ||
		payload.HostConfig.RestartPolicy.Name != "unless-stopped" ||
		payload.HostConfig.PortBindings["80/tcp"][0]["HostIp"] != "192.0.2.10" ||
		len(payload.HostConfig.Mounts) != 2 ||
		payload.HostConfig.Mounts[0].Type != "bind" ||
		payload.HostConfig.Mounts[0].Source != "/home/docker/demo" ||
		payload.HostConfig.Mounts[1].Type != "volume" ||
		!payload.HostConfig.Mounts[1].ReadOnly {
		t.Fatalf("unexpected create payload: %#v", payload)
	}
}

func TestManagedContainerCreatePullsMissingImageAndAllowsAutomaticName(t *testing.T) {
	createdID := strings.Repeat("e", 64)
	createCalls := 0
	pulled := false
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/containers/create":
			createCalls++
			if request.URL.Query().Get("name") != "" {
				t.Fatalf("automatic container name query = %q", request.URL.RawQuery)
			}
			if createCalls == 1 {
				response.WriteHeader(http.StatusNotFound)
				_, _ = response.Write([]byte(`{"message":"No such image: nginx:alpine"}`))
				return
			}
			_, _ = response.Write([]byte(`{"Id":"` + createdID + `"}`))
		case request.Method == http.MethodPost && request.URL.Path == "/images/create":
			pulled = request.URL.Query().Get("fromImage") == "nginx" && request.URL.Query().Get("tag") == "alpine"
			_, _ = response.Write([]byte("{}\n"))
		case request.Method == http.MethodPost && request.URL.Path == "/containers/"+createdID+"/start":
			response.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	client := testHTTPClient(server)
	if err := client.createManagedContainer(context.Background(), MaintenanceInput{
		Action: "container_create", Image: "nginx:alpine",
	}); err != nil {
		t.Fatal(err)
	}
	if createCalls != 2 || !pulled {
		t.Fatalf("create calls=%d pulled=%v", createCalls, pulled)
	}
}

func TestContainerExecFailsFastWhenConsoleSlotsAreFull(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("busy console request reached Docker")
	}))
	defer server.Close()
	client := testHTTPClient(server)
	for range maxConcurrentContainerExecs {
		containerExecSlots <- struct{}{}
	}
	defer func() {
		for range maxConcurrentContainerExecs {
			<-containerExecSlots
		}
	}()
	_, err := client.ContainerExec(context.Background(), strings.Repeat("a", 64),
		ContainerExecInput{ResourceVersion: "sha256:" + strings.Repeat("0", 64), Command: "id"})
	if !errors.Is(err, ErrContainerExecBusy) {
		t.Fatalf("busy console error = %v", err)
	}
	if _, err := client.ContainerExec(context.Background(), "not-an-id", ContainerExecInput{Command: "id"}); !errors.Is(err, ErrInvalidDockerJob) {
		t.Fatalf("invalid container ID error = %v", err)
	}
}
