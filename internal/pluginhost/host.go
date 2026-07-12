package pluginhost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"postizer/internal/appearance"
	"postizer/pkg/pluginrpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	startupTimeout           = 60 * time.Second
	callTimeout              = 2 * time.Minute
	maxPluginRPCMessageBytes = 96 << 20
)

type Host struct {
	appRoot     string
	dataRoot    string
	hostService pluginrpc.HostServiceServer

	mu      sync.Mutex
	clients map[string]*client
}

type client struct {
	pack         appearance.Pack
	cmd          *exec.Cmd
	done         <-chan struct{}
	conn         *grpc.ClientConn
	service      pluginrpc.PluginServiceClient
	hostServer   *grpc.Server
	hostListener net.Listener
}

type startupLine struct {
	Protocol string `json:"protocol"`
	Endpoint string `json:"endpoint"`
}

type runtimeCommand struct {
	command string
	args    []string
	workDir string
	env     map[string]string
}

func New(appRoot string, hostService ...pluginrpc.HostServiceServer) *Host {
	if strings.TrimSpace(appRoot) == "" {
		appRoot = "."
	}
	var service pluginrpc.HostServiceServer
	if len(hostService) > 0 {
		service = hostService[0]
	}
	return &Host{appRoot: appRoot, hostService: service, clients: map[string]*client{}}
}

// SetDataRoot configures a persistent directory outside installed bundle
// files. Each plugin receives an isolated child directory through
// POSTIZER_PLUGIN_DATA_DIR.
func (h *Host) SetDataRoot(root string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dataRoot = strings.TrimSpace(root)
}

func (h *Host) InvokeAction(ctx context.Context, pack appearance.Pack, req *pluginrpc.InvokeActionRequest) (*pluginrpc.InvokeActionResponse, error) {
	service, err := h.client(ctx, pack)
	if err != nil {
		return nil, err
	}
	if req.PluginID == "" {
		req.PluginID = pack.ID
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	return service.InvokeAction(ctx, req)
}

func (h *Host) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, client := range h.clients {
		closeClient(client)
		delete(h.clients, id)
	}
}

func (h *Host) client(ctx context.Context, pack appearance.Pack) (pluginrpc.PluginServiceClient, error) {
	if pack.Runtime.Kind != appearance.RuntimeGRPC {
		return nil, fmt.Errorf("plugin %q does not declare a grpc runtime", pack.ID)
	}

	h.mu.Lock()
	if existing := h.clients[pack.ID]; existing != nil {
		if pluginRunning(existing) {
			service := existing.service
			h.mu.Unlock()
			return service, nil
		}
		closeClient(existing)
		delete(h.clients, pack.ID)
	}
	h.mu.Unlock()

	started, err := h.start(ctx, pack)
	if err != nil {
		return nil, err
	}

	h.mu.Lock()
	h.clients[pack.ID] = started
	h.mu.Unlock()
	return started.service, nil
}

func (h *Host) start(ctx context.Context, pack appearance.Pack) (*client, error) {
	runtimeCommand, err := h.command(pack)
	if err != nil {
		return nil, err
	}
	command := h.resolveCommand(pack, runtimeCommand.command)
	dataDir, err := h.pluginDataDir(pack.ID)
	if err != nil {
		return nil, err
	}
	if err := ensureExecutable(command, runtimeCommand.command); err != nil {
		return nil, fmt.Errorf("prepare plugin executable %q: %w", pack.ID, err)
	}
	hostServer, hostListener, hostEndpoint, err := h.startHostService()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(command, runtimeCommand.args...)
	cmd.Dir = h.workDir(pack, runtimeCommand.workDir)
	cmd.Env = h.env(pack, runtimeCommand.env, command, hostEndpoint, dataDir)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open plugin stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open plugin stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		stopHostService(hostServer, hostListener)
		return nil, fmt.Errorf("start plugin %q: %w", pack.ID, err)
	}
	processDone := waitForProcess(cmd)
	go logPluginStderr(pack.ID, stderr)

	endpoint, err := readEndpoint(stdout)
	if err != nil {
		stopProcess(cmd, processDone)
		stopHostService(hostServer, hostListener)
		return nil, fmt.Errorf("start plugin %q: %w", pack.ID, err)
	}

	dialCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	conn, err := grpc.NewClient(
		endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(pluginrpc.Codec),
			grpc.MaxCallRecvMsgSize(maxPluginRPCMessageBytes),
			grpc.MaxCallSendMsgSize(maxPluginRPCMessageBytes),
		),
	)
	if err != nil {
		stopProcess(cmd, processDone)
		stopHostService(hostServer, hostListener)
		return nil, fmt.Errorf("dial plugin %q: %w", pack.ID, err)
	}
	if err := waitForReady(dialCtx, endpoint); err != nil {
		_ = conn.Close()
		stopProcess(cmd, processDone)
		stopHostService(hostServer, hostListener)
		return nil, fmt.Errorf("connect plugin %q: %w", pack.ID, err)
	}

	service := pluginrpc.NewPluginServiceClient(conn)
	handshake, err := service.Handshake(dialCtx, &pluginrpc.HandshakeRequest{
		ProtocolVersion: pluginrpc.ProtocolVersion,
		HostVersion:     "postizer-dev",
		PluginID:        pack.ID,
	})
	if err != nil {
		_ = conn.Close()
		stopProcess(cmd, processDone)
		stopHostService(hostServer, hostListener)
		return nil, fmt.Errorf("handshake plugin %q: %w", pack.ID, err)
	}
	if handshake.ProtocolVersion != pluginrpc.ProtocolVersion || !handshake.Ready {
		_ = conn.Close()
		stopProcess(cmd, processDone)
		stopHostService(hostServer, hostListener)
		return nil, fmt.Errorf("plugin %q rejected protocol: %s", pack.ID, handshake.Message)
	}

	return &client{pack: pack, cmd: cmd, done: processDone, conn: conn, service: service, hostServer: hostServer, hostListener: hostListener}, nil
}

func (h *Host) startHostService() (*grpc.Server, net.Listener, string, error) {
	if h.hostService == nil {
		return nil, nil, "", nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, "", fmt.Errorf("listen host service: %w", err)
	}
	server := grpc.NewServer(
		grpc.ForceServerCodec(pluginrpc.Codec),
		grpc.MaxRecvMsgSize(maxPluginRPCMessageBytes),
		grpc.MaxSendMsgSize(maxPluginRPCMessageBytes),
	)
	pluginrpc.RegisterHostServiceServer(server, h.hostService)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			log.Printf("plugin host service: %v", err)
		}
	}()
	return server, listener, listener.Addr().String(), nil
}

func (h *Host) command(pack appearance.Pack) (runtimeCommand, error) {
	selected := runtimeCommand{
		command: strings.TrimSpace(pack.Runtime.Command),
		args:    append([]string(nil), pack.Runtime.Args...),
		workDir: strings.TrimSpace(pack.Runtime.WorkDir),
		env:     cloneEnv(pack.Runtime.Env),
	}
	for _, platform := range pack.Runtime.Platforms {
		if platform.GOOS != runtime.GOOS || platform.GOArch != runtime.GOARCH {
			continue
		}
		selected.command = strings.TrimSpace(platform.Command)
		if platform.Args != nil {
			selected.args = append([]string(nil), platform.Args...)
		}
		if strings.TrimSpace(platform.WorkDir) != "" {
			selected.workDir = strings.TrimSpace(platform.WorkDir)
		}
		selected.env = mergeEnv(selected.env, platform.Env)
		break
	}
	if selected.command == "" {
		if len(pack.Runtime.Platforms) > 0 {
			return runtimeCommand{}, fmt.Errorf("plugin %q does not provide a runtime for %s/%s", pack.ID, runtime.GOOS, runtime.GOARCH)
		}
		return runtimeCommand{}, errors.New("runtime.command is required")
	}
	return selected, nil
}

func (h *Host) resolveCommand(pack appearance.Pack, command string) string {
	if command == "${go}" {
		if value := strings.TrimSpace(os.Getenv("POSTIZER_GO")); value != "" {
			return value
		}
		if runtime.GOOS == "windows" {
			candidate := filepath.Join(os.Getenv("ProgramFiles"), "Go", "bin", "go.exe")
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		return "go"
	}
	if filepath.IsAbs(command) {
		return command
	}
	if strings.ContainsAny(command, `/\`) {
		return h.absPath(filepath.Join(pack.RootDir, filepath.FromSlash(command)))
	}
	return command
}

func (h *Host) workDir(pack appearance.Pack, configured string) string {
	workDir := strings.TrimSpace(configured)
	if workDir == "" {
		return h.absPath(pack.RootDir)
	}
	if filepath.IsAbs(workDir) {
		return workDir
	}
	return h.absPath(filepath.Join(pack.RootDir, filepath.FromSlash(workDir)))
}

func (h *Host) env(pack appearance.Pack, runtimeEnv map[string]string, command, hostEndpoint, dataDir string) []string {
	env := os.Environ()
	env = append(env,
		"POSTIZER_PLUGIN_ADDR=127.0.0.1:0",
		"POSTIZER_PLUGIN_ID="+pack.ID,
		"POSTIZER_PLUGIN_ROOT="+h.absPath(pack.RootDir),
	)
	if dataDir != "" {
		env = append(env, "POSTIZER_PLUGIN_DATA_DIR="+dataDir)
	}
	if strings.TrimSpace(hostEndpoint) != "" {
		env = append(env, "POSTIZER_HOST_ADDR="+hostEndpoint)
	}
	for key, value := range runtimeEnv {
		env = append(env, key+"="+value)
	}
	if strings.EqualFold(filepath.Base(command), "go.exe") || filepath.Base(command) == "go" {
		if runtime.GOOS == "windows" {
			goroot := filepath.Join(os.Getenv("ProgramFiles"), "Go")
			if _, err := os.Stat(goroot); err == nil {
				env = setEnv(env, "GOROOT", goroot)
			}
		}
	}
	return env
}

func (h *Host) pluginDataDir(pluginID string) (string, error) {
	root := strings.TrimSpace(h.dataRoot)
	if root == "" {
		return "", nil
	}
	dir := filepath.Join(root, pluginID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create plugin data directory %q: %w", pluginID, err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve plugin data directory %q: %w", pluginID, err)
	}
	return abs, nil
}

func (h *Host) absPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolute
}

func readEndpoint(stdout io.Reader) (string, error) {
	type result struct {
		endpoint string
		err      error
	}
	ch := make(chan result, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1024), 1<<20)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				ch <- result{err: err}
				return
			}
			ch <- result{err: errors.New("plugin exited before announcing endpoint")}
			return
		}
		var line startupLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			ch <- result{err: fmt.Errorf("parse startup line %q: %w", scanner.Text(), err)}
			return
		}
		if line.Protocol != pluginrpc.ProtocolVersion {
			ch <- result{err: fmt.Errorf("unexpected startup protocol %q", line.Protocol)}
			return
		}
		if strings.TrimSpace(line.Endpoint) == "" {
			ch <- result{err: errors.New("plugin did not announce an endpoint")}
			return
		}
		ch <- result{endpoint: line.Endpoint}
	}()
	select {
	case res := <-ch:
		return res.endpoint, res.err
	case <-time.After(startupTimeout):
		return "", errors.New("timed out waiting for plugin endpoint")
	}
}

func waitForReady(ctx context.Context, endpoint string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		conn, err := net.DialTimeout("tcp", endpoint, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func pluginRunning(client *client) bool {
	if client == nil || client.cmd == nil || client.cmd.Process == nil {
		return false
	}
	select {
	case <-client.done:
		return false
	default:
		return true
	}
}

func waitForProcess(cmd *exec.Cmd) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	return done
}

func closeClient(client *client) {
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if client.service != nil {
		_, _ = client.service.Shutdown(ctx, &pluginrpc.ShutdownRequest{PluginID: client.pack.ID})
	}
	if client.conn != nil {
		_ = client.conn.Close()
	}
	stopHostService(client.hostServer, client.hostListener)
	stopProcess(client.cmd, client.done)
}

func stopProcess(cmd *exec.Cmd, done <-chan struct{}) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	select {
	case <-done:
		return
	default:
	}
	_ = cmd.Process.Kill()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func stopHostService(server *grpc.Server, listener net.Listener) {
	if server != nil {
		server.Stop()
	}
	if listener != nil {
		_ = listener.Close()
	}
}

func logPluginStderr(id string, stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		log.Printf("plugin %s: %s", id, scanner.Text())
	}
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for index, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[index] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func cloneEnv(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	cloned := map[string]string{}
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func mergeEnv(base, overlay map[string]string) map[string]string {
	if len(overlay) == 0 {
		return base
	}
	merged := cloneEnv(base)
	if merged == nil {
		merged = map[string]string{}
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func ensureExecutable(resolvedCommand, manifestCommand string) error {
	if runtime.GOOS == "windows" || !strings.ContainsAny(manifestCommand, `/\`) {
		return nil
	}
	info, err := os.Stat(resolvedCommand)
	if err != nil {
		return err
	}
	if info.IsDir() || info.Mode()&0111 != 0 {
		return nil
	}
	return os.Chmod(resolvedCommand, info.Mode()|0755)
}
