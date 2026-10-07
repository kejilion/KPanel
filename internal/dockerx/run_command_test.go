package dockerx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func decodeRunInspect(t *testing.T, raw string) runInspect {
	t.Helper()
	var value runInspect
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func decodeRunImage(t *testing.T, raw string) *runImageConfig {
	t.Helper()
	var value runImageConfig
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return &value
}

func runOptionText(options []RunOption) []string {
	result := make([]string, 0, len(options))
	for _, option := range options {
		if option.Value == nil {
			result = append(result, option.Flag)
		} else {
			result = append(result, option.Flag+" "+*option.Value)
		}
	}
	return result
}

var testRunDaemon = runDaemonDefaults{LogDriver: "json-file", Runtime: "runc", LogOpts: map[string]string{}}

func TestBuildRunCommandOmitsWhatTheImageAlreadySupplies(t *testing.T) {
	id := "0123456789ab" + strings.Repeat("c", 52)
	container := decodeRunInspect(t, `{
		"Id":"`+id+`","Name":"/web","Image":"sha256:feed",
		"Config":{
			"Hostname":"0123456789ab","Image":"nginx:alpine",
			"Env":["PATH=/usr/local/sbin:/usr/bin","NGINX_VERSION=1.27","TZ=Asia/Shanghai"],
			"Cmd":["nginx","-g","daemon off;"],"Entrypoint":["/docker-entrypoint.sh"],
			"WorkingDir":"/","Labels":{"maintainer":"NGINX","com.docker.compose.project":"x"},
			"ExposedPorts":{"80/tcp":{}},"Volumes":{"/var/cache/nginx":{}},
			"StopSignal":"SIGQUIT"
		},
		"HostConfig":{
			"NetworkMode":"bridge","RestartPolicy":{"Name":"no"},
			"PortBindings":{"80/tcp":[{"HostIp":"","HostPort":"8080"}]},
			"LogConfig":{"Type":"json-file","Config":{}},"ShmSize":67108864,"IpcMode":"private",
			"CpuShares":0,"MemorySwappiness":null
		},
		"NetworkSettings":{"Networks":{"bridge":{"IPAMConfig":null,"Aliases":null}}}
	}`)
	image := decodeRunImage(t, `{
		"Env":["PATH=/usr/local/sbin:/usr/bin","NGINX_VERSION=1.27"],
		"Cmd":["nginx","-g","daemon off;"],"Entrypoint":["/docker-entrypoint.sh"],
		"Labels":{"maintainer":"NGINX"},"ExposedPorts":{"80/tcp":{}},
		"Volumes":{"/var/cache/nginx":{}},"StopSignal":"SIGQUIT"
	}`)
	collected := time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC)
	result := buildRunCommand(container, image, testRunDaemon, collected)
	want := []string{"-d", "--name web", "-p 8080:80", "-e TZ=Asia/Shanghai"}
	if got := runOptionText(result.Options); !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %#v, want %#v", got, want)
	}
	if result.Image != "nginx:alpine" || len(result.Command) != 0 || len(result.Networks) != 0 ||
		len(result.Unsupported) != 0 || !result.ImageDefaults || result.ComposeProject != "x" ||
		!result.CollectedAt.Equal(collected) {
		t.Fatalf("unexpected reconstruction: %#v", result)
	}
}

func TestBuildRunCommandReconstructsExplicitOptions(t *testing.T) {
	id := strings.Repeat("d", 64)
	container := decodeRunInspect(t, `{
		"Id":"`+id+`","Name":"/app-1","Image":"sha256:beef",
		"Config":{
			"Hostname":"app","Domainname":"example.test","User":"1000:1000","Tty":true,"OpenStdin":true,
			"Image":"ghcr.io/acme/app:2","Env":["PATH=/bin","DB_PASSWORD=s3cret value"],
			"Cmd":["--port","9000"],"Entrypoint":["/bin/app","serve"],"WorkingDir":"/srv",
			"Labels":{"traefik.enable":"true","com.docker.compose.service":"app","io.kejilion.panel.managed":"true","version":"2"},
			"ExposedPorts":{"9000/tcp":{},"9100/tcp":{},"53/udp":{}},
			"Volumes":{"/data":{},"/scratch":{}},
			"StopSignal":"SIGTERM","StopTimeout":30,
			"Healthcheck":{"Test":["CMD-SHELL","curl -f http://localhost:9000/ || exit 1"],"Interval":30000000000,"Retries":5},
			"MacAddress":"02:42:ac:11:00:02"
		},
		"HostConfig":{
			"Binds":["/srv/app/config:/etc/app:ro","appdata:/var/lib/app"],
			"Mounts":[
				{"Type":"bind","Source":"/srv/app/logs","Target":"/logs"},
				{"Type":"volume","Source":"cache","Target":"/cache","ReadOnly":true},
				{"Type":"volume","Source":"seed","Target":"/seed","VolumeOptions":{"NoCopy":true}}
			],
			"Tmpfs":{"/run":"rw,size=64m"},
			"NetworkMode":"backend",
			"PortBindings":{
				"9000/tcp":[{"HostIp":"127.0.0.1","HostPort":"9000"}],
				"53/udp":[{"HostIp":"","HostPort":"5353"}],
				"8000/tcp":[{"HostIp":"","HostPort":"18000"}],
				"8001/tcp":[{"HostIp":"","HostPort":"18001"}],
				"8002/tcp":[{"HostIp":"","HostPort":"18002"}],
				"7000/tcp":[{"HostIp":"::1","HostPort":"7000"}],
				"6000/tcp":[]
			},
			"RestartPolicy":{"Name":"on-failure","MaximumRetryCount":3},
			"Privileged":true,"CapAdd":["CAP_NET_ADMIN"],"CapDrop":["MKNOD"],
			"Devices":[{"PathOnHost":"/dev/net/tun","PathInContainer":"/dev/net/tun","CgroupPermissions":"rwm"},
				{"PathOnHost":"/dev/sda","PathInContainer":"/dev/xvda","CgroupPermissions":"r"}],
			"DeviceRequests":[{"Driver":"","Count":-1,"Capabilities":[["gpu"]]}],
			"SecurityOpt":["apparmor=unconfined","label=disable"],"PidMode":"host","IpcMode":"shareable",
			"Memory":536870912,"MemorySwap":1073741824,"NanoCpus":1500000000,"CpuShares":512,
			"PidsLimit":256,"ShmSize":268435456,
			"Ulimits":[{"Name":"nofile","Soft":65535,"Hard":65535},{"Name":"nproc","Soft":1024,"Hard":2048}],
			"Sysctls":{"net.core.somaxconn":"1024"},
			"ExtraHosts":["host.docker.internal:host-gateway"],"Dns":["1.1.1.1"],
			"LogConfig":{"Type":"json-file","Config":{"max-size":"20m","max-file":"5"}},
			"Init":true,"ReadonlyRootfs":true,"Runtime":"runc","StorageOpt":{"size":"10G"},
			"Links":["/db:/app-1/database"]
		},
		"NetworkSettings":{"Networks":{
			"backend":{"IPAMConfig":{"IPv4Address":"172.30.0.10"},"Aliases":["app-1","app","dddddddddddd"]},
			"frontend":{"IPAMConfig":null,"Aliases":["web"]}
		}}
	}`)
	image := decodeRunImage(t, `{
		"Env":["PATH=/bin"],"Cmd":["serve"],"Entrypoint":["/bin/app"],"WorkingDir":"/",
		"Labels":{"version":"1"},"ExposedPorts":{"9100/tcp":{}},"Volumes":{"/data":{}}
	}`)
	daemon := runDaemonDefaults{LogDriver: "json-file", Runtime: "runc", LogOpts: map[string]string{"max-size": "20m"}}
	result := buildRunCommand(container, image, daemon, time.Unix(0, 0))
	want := []string{
		"-d", "-it", "--name app-1", "--hostname app", "--domainname example.test",
		"--restart on-failure:3",
		"--network backend", "--ip 172.30.0.10", "--network-alias app",
		"--link db:database",
		"-p 6000", "-p 18000-18002:8000-8002", "-p 127.0.0.1:9000:9000", "-p [::1]:7000:7000", "-p 5353:53/udp",
		"-v /srv/app/config:/etc/app:ro", "-v appdata:/var/lib/app",
		"-v /srv/app/logs:/logs", "-v cache:/cache:ro", "--mount type=volume,source=seed,target=/seed,volume-nocopy",
		"--tmpfs /run:rw,size=64m", "-v /scratch",
		"-e DB_PASSWORD=s3cret value",
		"--label traefik.enable=true", "--label version=2",
		"--user 1000:1000", "--workdir /srv", "--entrypoint /bin/app",
		"--privileged", "--cap-add NET_ADMIN", "--cap-drop MKNOD",
		"--device /dev/net/tun", "--device /dev/sda:/dev/xvda:r", "--gpus all",
		"--security-opt apparmor=unconfined", "--pid host",
		"--memory 512m", "--cpus 1.5", "--cpu-shares 512", "--pids-limit 256", "--shm-size 256m",
		"--ulimit nofile=65535", "--ulimit nproc=1024:2048", "--sysctl net.core.somaxconn=1024",
		"--add-host host.docker.internal:host-gateway", "--dns 1.1.1.1",
		"--log-opt max-file=5",
		"--health-cmd curl -f http://localhost:9000/ || exit 1", "--health-interval 30s", "--health-retries 5",
		"--stop-signal SIGTERM", "--stop-timeout 30", "--init", "--read-only",
	}
	if got := runOptionText(result.Options); !reflect.DeepEqual(got, want) {
		t.Fatalf("options:\n got %#v\nwant %#v", got, want)
	}
	if !reflect.DeepEqual(result.Command, []string{"serve", "--port", "9000"}) {
		t.Fatalf("command = %#v", result.Command)
	}
	if !reflect.DeepEqual(result.Networks, []RunNetwork{{Name: "frontend", Aliases: []string{"web"}}}) {
		t.Fatalf("networks = %#v", result.Networks)
	}
	if !reflect.DeepEqual(result.Unsupported, []string{"--mac-address", "--storage-opt"}) {
		t.Fatalf("unsupported = %#v", result.Unsupported)
	}
}

func TestBuildRunCommandKeepsEntrypointThatSuppressesImageCommand(t *testing.T) {
	container := decodeRunInspect(t, `{"Id":"`+strings.Repeat("e", 64)+`","Name":"/x",
		"Config":{"Image":"busybox","Entrypoint":["sh"],"Cmd":null},"HostConfig":{"NetworkMode":"bridge"}}`)
	image := decodeRunImage(t, `{"Entrypoint":["sh"],"Cmd":["-c","echo hi"]}`)
	result := buildRunCommand(container, image, testRunDaemon, time.Unix(0, 0))
	if got := runOptionText(result.Options); !reflect.DeepEqual(got, []string{"-d", "--name x", "--entrypoint sh"}) {
		t.Fatalf("options = %#v", got)
	}

	cleared := decodeRunInspect(t, `{"Id":"`+strings.Repeat("e", 64)+`","Name":"/x",
		"Config":{"Image":"busybox","Entrypoint":[""],"Cmd":["true"]},"HostConfig":{"NetworkMode":"bridge"}}`)
	result = buildRunCommand(cleared, image, testRunDaemon, time.Unix(0, 0))
	last := result.Options[len(result.Options)-1]
	if last.Flag != "--entrypoint" || last.Value == nil || *last.Value != "" ||
		!reflect.DeepEqual(result.Command, []string{"true"}) {
		t.Fatalf("cleared entrypoint = %#v command %#v", last, result.Command)
	}
	encoded, err := json.Marshal(result.Options)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `{"flag":"-d"}`) || !strings.Contains(string(encoded), `{"flag":"--entrypoint","value":""}`) {
		t.Fatalf("switches and empty values must stay distinguishable: %s", encoded)
	}
}

func TestBuildRunCommandSkipsSharedNamespacesAndDaemonDefaults(t *testing.T) {
	container := decodeRunInspect(t, `{"Id":"`+strings.Repeat("f", 64)+`","Name":"/probe",
		"Config":{"Hostname":"my-host","Image":"probe:1"},
		"HostConfig":{"NetworkMode":"host","LogConfig":{"Type":"local","Config":{"max-size":"20m"}},
			"Runtime":"nvidia","MemorySwap":-1,"Memory":0,"IpcMode":"host"},
		"NetworkSettings":{"Networks":{"host":{"Aliases":null}}}}`)
	daemon := runDaemonDefaults{LogDriver: "json-file", Runtime: "runc", LogOpts: map[string]string{"max-size": "20m"}}
	result := buildRunCommand(container, nil, daemon, time.Unix(0, 0))
	want := []string{"-d", "--name probe", "--network host", "--ipc host", "--memory-swap -1",
		"--log-driver local", "--log-opt max-size=20m", "--runtime nvidia"}
	if got := runOptionText(result.Options); !reflect.DeepEqual(got, want) {
		t.Fatalf("options:\n got %#v\nwant %#v", got, want)
	}
	if result.ImageDefaults || len(result.Networks) != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestContainerRunCommandReadsContainerImageAndDaemonDefaults(t *testing.T) {
	id := strings.Repeat("a", 64)
	var (
		pathsMu sync.Mutex
		paths   []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		pathsMu.Lock()
		paths = append(paths, request.Method+" "+request.URL.EscapedPath())
		pathsMu.Unlock()
		switch request.URL.EscapedPath() {
		case "/containers/" + id + "/json":
			_, _ = response.Write([]byte(`{"Id":"` + id + `","Name":"/svc","Image":"sha256:` + id + `",
				"Config":{"Image":"svc:1","Env":["PATH=/bin","MODE=prod"]},
				"HostConfig":{"NetworkMode":"bridge","LogConfig":{"Type":"json-file","Config":{"max-size":"10m"}}}}`))
		case "/images/sha256:" + id + "/json":
			_, _ = response.Write([]byte(`{"Config":{"Env":["PATH=/bin"]}}`))
		case "/info":
			_, _ = response.Write([]byte(`{"LoggingDriver":"json-file","DefaultRuntime":"runc"}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	client := testHTTPClient(server)
	client.daemonConfigPath = filepath.Join(t.TempDir(), "daemon.json")
	if err := os.WriteFile(client.daemonConfigPath, []byte(`{"log-opts":{"max-size":"10m"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := client.ContainerRunCommand(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got := runOptionText(result.Options); !reflect.DeepEqual(got, []string{"-d", "--name svc", "-e MODE=prod"}) || !result.ImageDefaults {
		t.Fatalf("result = %#v options %#v", result, got)
	}
	pathsMu.Lock()
	seen := append([]string(nil), paths...)
	pathsMu.Unlock()
	for _, path := range seen {
		if !strings.HasPrefix(path, "GET ") {
			t.Fatalf("run command must stay read-only, saw %s", path)
		}
	}

	if _, err := client.ContainerRunCommand(context.Background(), "not-an-id"); err == nil {
		t.Fatal("invalid container id was accepted")
	}
	_, err = client.ContainerRunCommand(context.Background(), strings.Repeat("b", 64))
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.Status != http.StatusNotFound {
		t.Fatalf("missing container error = %v", err)
	}
}

func TestContainerRunCommandFallsBackWhenImageIsUnreadable(t *testing.T) {
	id := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/containers/"+id+"/json" {
			_, _ = response.Write([]byte(`{"Id":"` + id + `","Name":"/svc","Image":"sha256:gone",
				"Config":{"Image":"svc:1","Env":["PATH=/bin"]},"HostConfig":{"NetworkMode":"bridge"}}`))
			return
		}
		http.NotFound(response, request)
	}))
	defer server.Close()
	result, err := testHTTPClient(server).ContainerRunCommand(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if result.ImageDefaults || !reflect.DeepEqual(runOptionText(result.Options), []string{"-d", "--name svc", "-e PATH=/bin"}) {
		t.Fatalf("fallback result = %#v", result)
	}
}

func TestBuildRunCommandQuotesCSVValuesAndResolvesNetworkIDs(t *testing.T) {
	id := strings.Repeat("9", 64)
	container := decodeRunInspect(t, `{"Id":"`+id+`","Name":"/gpu",
		"Config":{"Image":"cuda:12"},
		"HostConfig":{"NetworkMode":"0123456789abcdef",
			"DeviceRequests":[{"Driver":"nvidia","Count":0,"DeviceIDs":["0","1"],"Capabilities":[["gpu"]]}],
			"Mounts":[{"Type":"volume","Source":"nfs","Target":"/data","VolumeOptions":{"DriverConfig":{"Name":"local",
				"Options":{"device":":/export","o":"addr=10.0.0.1,rw","type":"nfs"}}}}]},
		"NetworkSettings":{"Networks":{
			"backend":{"NetworkID":"0123456789abcdef0000","IPAMConfig":{"IPv4Address":"172.30.0.9"},"Aliases":["gpu","api"]},
			"monitoring":{"NetworkID":"fedcba9876543210"}
		}}}`)
	result := buildRunCommand(container, &runImageConfig{}, testRunDaemon, time.Unix(0, 0))
	want := []string{
		"-d", "--name gpu", "--network backend", "--ip 172.30.0.9", "--network-alias api",
		`--mount type=volume,source=nfs,target=/data,volume-driver=local,volume-opt=device=:/export,"volume-opt=o=addr=10.0.0.1,rw",volume-opt=type=nfs`,
		`--gpus "device=0,1"`,
	}
	if got := runOptionText(result.Options); !reflect.DeepEqual(got, want) {
		t.Fatalf("options:\n got %#v\nwant %#v", got, want)
	}
	if !reflect.DeepEqual(result.Networks, []RunNetwork{{Name: "monitoring", Aliases: []string{}}}) {
		t.Fatalf("networks = %#v", result.Networks)
	}
}

func TestByteSizeOptionUsesExactUnits(t *testing.T) {
	for value, want := range map[int64]string{
		1 << 30: "1g", 3 << 29: "1536m", 64 << 20: "64m", 2048: "2k", 1536: "1536b", 1000: "1000b",
	} {
		if got := byteSizeOption(value); got != want {
			t.Fatalf("byteSizeOption(%d) = %q, want %q", value, got, want)
		}
	}
}
