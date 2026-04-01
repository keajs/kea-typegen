package tsgoapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const signatureKindCall = 0

type InitializeResponse struct {
	UseCaseSensitiveFileNames bool   `json:"useCaseSensitiveFileNames"`
	CurrentDirectory          string `json:"currentDirectory"`
}

type ConfigResponse struct {
	Options   map[string]any `json:"options"`
	FileNames []string       `json:"fileNames"`
}

type UpdateSnapshotResponse struct {
	Snapshot string            `json:"snapshot"`
	Projects []ProjectResponse `json:"projects"`
}

type ProjectResponse struct {
	ID              string         `json:"id"`
	ConfigFileName  string         `json:"configFileName"`
	CompilerOptions map[string]any `json:"compilerOptions"`
	RootFiles       []string       `json:"rootFiles"`
}

type SymbolResponse struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Flags            uint32   `json:"flags"`
	CheckFlags       uint32   `json:"checkFlags"`
	Declarations     []string `json:"declarations,omitempty"`
	ValueDeclaration string   `json:"valueDeclaration,omitempty"`
}

type TypeResponse struct {
	ID          string `json:"id"`
	Flags       uint32 `json:"flags"`
	ObjectFlags uint32 `json:"objectFlags,omitempty"`
	Symbol      string `json:"symbol,omitempty"`
}

type SignatureResponse struct {
	ID             string   `json:"id"`
	Flags          uint32   `json:"flags"`
	Declaration    string   `json:"declaration,omitempty"`
	TypeParameters []string `json:"typeParameters,omitempty"`
	Parameters     []string `json:"parameters,omitempty"`
	ThisParameter  string   `json:"thisParameter,omitempty"`
	Target         string   `json:"target,omitempty"`
}

type base64Data struct {
	Data string `json:"data"`
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Client struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	reader    *bufio.Reader
	stderr    bytes.Buffer
	nextID    atomic.Int64
	pendingMu sync.Mutex
	pending   map[string]chan rpcEnvelope
	closed    chan struct{}
	closeOnce sync.Once
	waitDone  chan error
	exitErrMu sync.Mutex
	exitErr   error
}

func PreferredBinary(repoRoot string) string {
	for _, candidate := range candidateBinaries(repoRoot) {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	for _, name := range []string{"tsgo", "tsgo-upstream"} {
		if resolved, err := exec.LookPath(name); err == nil {
			return resolved
		}
	}
	return candidateBinaries(repoRoot)[0]
}

func candidateBinaries(repoRoot string) []string {
	return []string{
		filepath.Join(repoRoot, ".tsgo", "node_modules", ".bin", "tsgo"),
		filepath.Join(repoRoot, "node_modules", ".bin", "tsgo"),
		filepath.Join(repoRoot, "poc", "tsgo-lsp", "bin", "tsgo-upstream"),
		filepath.Join(repoRoot, "poc", "tsgo-api", "bin", "tsgo-upstream"),
		filepath.Join(repoRoot, "poc", "tsgo-lsp", "node_modules", ".bin", "tsgo"),
	}
}

func Preflight(binaryPath, projectDir string) error {
	resolvedBinary, err := resolveBinaryPath(binaryPath)
	if err != nil {
		return err
	}

	cmd := exec.Command(resolvedBinary, "--api", "--help")
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	text := string(output)
	if strings.Contains(text, "Usage of api:") || strings.Contains(text, "use JSON-RPC protocol instead of MessagePack") {
		return nil
	}
	if err != nil && strings.TrimSpace(text) == "" {
		return fmt.Errorf("tsgo binary at %s failed `--api --help`: %w", resolvedBinary, err)
	}
	return fmt.Errorf(
		"tsgo binary at %s does not expose a usable `--api` entrypoint; output was:\n%s",
		resolvedBinary,
		strings.TrimSpace(text),
	)
}

func Start(projectDir, binaryPath string) (*Client, error) {
	resolvedBinary, err := resolveBinaryPath(binaryPath)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(resolvedBinary, "--api", "--async", "--cwd", projectDir)
	cmd.Dir = projectDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	client := &Client{
		cmd:      cmd,
		stdin:    stdin,
		reader:   bufio.NewReader(stdout),
		pending:  make(map[string]chan rpcEnvelope),
		closed:   make(chan struct{}),
		waitDone: make(chan error, 1),
	}

	go func() {
		_, _ = io.Copy(&client.stderr, stderr)
	}()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start tsgo api: %w", err)
	}

	go func() {
		err := cmd.Wait()
		if err != nil {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(client.stderr.String()))
		}
		client.setExitError(err)
		client.closeOnce.Do(func() {
			close(client.closed)
		})
		client.waitDone <- err
		close(client.waitDone)
	}()

	go client.readLoop()
	return client, nil
}

func resolveBinaryPath(binaryPath string) (string, error) {
	binaryPath = strings.TrimSpace(binaryPath)
	if binaryPath == "" {
		return "", fmt.Errorf("tsgo binary path is empty")
	}
	if filepath.IsAbs(binaryPath) || strings.Contains(binaryPath, string(os.PathSeparator)) {
		if _, err := os.Stat(binaryPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("tsgo binary not found at %s", binaryPath)
			}
			return "", err
		}
		return binaryPath, nil
	}
	resolved, err := exec.LookPath(binaryPath)
	if err != nil {
		return "", fmt.Errorf("tsgo binary %q not found in PATH", binaryPath)
	}
	return resolved, nil
}

func (c *Client) Close() error {
	_ = c.stdin.Close()
	select {
	case err := <-c.waitDone:
		return err
	case <-time.After(2 * time.Second):
		_ = c.cmd.Process.Kill()
		return <-c.waitDone
	}
}

func (c *Client) Initialize(ctx context.Context) (*InitializeResponse, error) {
	var response InitializeResponse
	if err := c.call(ctx, "initialize", nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) ParseConfigFile(ctx context.Context, configFile string) (*ConfigResponse, error) {
	var response ConfigResponse
	if err := c.call(ctx, "parseConfigFile", map[string]any{"file": configFile}, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) UpdateSnapshot(ctx context.Context, configFile string) (*UpdateSnapshotResponse, error) {
	var response UpdateSnapshotResponse
	if err := c.call(ctx, "updateSnapshot", map[string]any{"openProject": configFile}, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) GetDefaultProjectForFile(ctx context.Context, snapshot, file string) (*ProjectResponse, error) {
	var response ProjectResponse
	if err := c.call(ctx, "getDefaultProjectForFile", map[string]any{
		"snapshot": snapshot,
		"file":     file,
	}, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) GetSymbolAtPosition(ctx context.Context, snapshot, projectID, file string, position int) (*SymbolResponse, error) {
	var response *SymbolResponse
	if err := c.call(ctx, "getSymbolAtPosition", map[string]any{
		"snapshot": snapshot,
		"project":  projectID,
		"file":     file,
		"position": position,
	}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) GetTypeOfSymbol(ctx context.Context, snapshot, projectID, symbolID string) (*TypeResponse, error) {
	var response *TypeResponse
	if err := c.call(ctx, "getTypeOfSymbol", map[string]any{
		"snapshot": snapshot,
		"project":  projectID,
		"symbol":   symbolID,
	}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) GetDeclaredTypeOfSymbol(ctx context.Context, snapshot, projectID, symbolID string) (*TypeResponse, error) {
	var response *TypeResponse
	if err := c.call(ctx, "getDeclaredTypeOfSymbol", map[string]any{
		"snapshot": snapshot,
		"project":  projectID,
		"symbol":   symbolID,
	}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) GetMembersOfSymbol(ctx context.Context, snapshot, projectID, symbolID string) ([]*SymbolResponse, error) {
	var response []*SymbolResponse
	if err := c.call(ctx, "getMembersOfSymbol", map[string]any{
		"snapshot": snapshot,
		"project":  projectID,
		"symbol":   symbolID,
	}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) GetTypeAtPosition(ctx context.Context, snapshot, projectID, file string, position int) (*TypeResponse, error) {
	var response *TypeResponse
	if err := c.call(ctx, "getTypeAtPosition", map[string]any{
		"snapshot": snapshot,
		"project":  projectID,
		"file":     file,
		"position": position,
	}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) GetSymbolOfType(ctx context.Context, snapshot, projectID, typeID string) (*SymbolResponse, error) {
	var response *SymbolResponse
	if err := c.call(ctx, "getSymbolOfType", map[string]any{
		"snapshot": snapshot,
		"project":  projectID,
		"type":     typeID,
	}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) GetTypeAtLocation(ctx context.Context, snapshot, projectID, location string) (*TypeResponse, error) {
	var response *TypeResponse
	if err := c.call(ctx, "getTypeAtLocation", map[string]any{
		"snapshot": snapshot,
		"project":  projectID,
		"location": location,
	}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) GetContextualType(ctx context.Context, snapshot, projectID, location string) (*TypeResponse, error) {
	var response *TypeResponse
	if err := c.call(ctx, "getContextualType", map[string]any{
		"snapshot": snapshot,
		"project":  projectID,
		"location": location,
	}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) TypeToString(ctx context.Context, snapshot, projectID, typeID string) (string, error) {
	var response string
	if err := c.call(ctx, "typeToString", map[string]any{
		"snapshot": snapshot,
		"project":  projectID,
		"type":     typeID,
	}, &response); err != nil {
		return "", err
	}
	return response, nil
}

func (c *Client) PrintTypeNode(ctx context.Context, snapshot, projectID, typeID string) (string, error) {
	return c.PrintTypeNodeAtLocation(ctx, snapshot, projectID, typeID, "")
}

func (c *Client) PrintTypeNodeAtLocation(ctx context.Context, snapshot, projectID, typeID, location string) (string, error) {
	var encoded *base64Data
	params := map[string]any{
		"snapshot": snapshot,
		"project":  projectID,
		"type":     typeID,
	}
	if strings.TrimSpace(location) != "" {
		params["location"] = location
	}
	if err := c.call(ctx, "typeToTypeNode", params, &encoded); err != nil {
		return "", err
	}
	if encoded == nil || encoded.Data == "" {
		return "", nil
	}

	var response string
	if err := c.call(ctx, "printNode", map[string]any{"data": encoded.Data}, &response); err != nil {
		return "", err
	}
	return response, nil
}

func (c *Client) GetPropertiesOfType(ctx context.Context, snapshot, projectID, typeID string) ([]*SymbolResponse, error) {
	var response []*SymbolResponse
	if err := c.call(ctx, "getPropertiesOfType", map[string]any{
		"snapshot": snapshot,
		"project":  projectID,
		"type":     typeID,
	}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) GetSignaturesOfType(ctx context.Context, snapshot, projectID, typeID string) ([]*SignatureResponse, error) {
	var response []*SignatureResponse
	if err := c.call(ctx, "getSignaturesOfType", map[string]any{
		"snapshot": snapshot,
		"project":  projectID,
		"type":     typeID,
		"kind":     signatureKindCall,
	}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) GetReturnTypeOfSignature(ctx context.Context, snapshot, projectID, signatureID string) (*TypeResponse, error) {
	var response *TypeResponse
	if err := c.call(ctx, "getReturnTypeOfSignature", map[string]any{
		"snapshot":  snapshot,
		"project":   projectID,
		"signature": signatureID,
	}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) CallRaw(ctx context.Context, method string, params any) (json.RawMessage, error) {
	var response json.RawMessage
	if err := c.call(ctx, method, params, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func PickProject(projects []ProjectResponse, configFile string) *ProjectResponse {
	for _, project := range projects {
		if project.ConfigFileName == configFile {
			projectCopy := project
			return &projectCopy
		}
	}
	if len(projects) == 0 {
		return nil
	}
	projectCopy := projects[0]
	return &projectCopy
}

func WithTimeout(ctx context.Context, timeout time.Duration) context.Context {
	deadlineCtx, _ := context.WithTimeout(ctx, timeout)
	return deadlineCtx
}

func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	id := c.nextID.Add(1)
	key := fmt.Sprintf("%d", id)
	responseCh := make(chan rpcEnvelope, 1)

	c.pendingMu.Lock()
	c.pending[key] = responseCh
	c.pendingMu.Unlock()

	if err := c.write(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, key)
		c.pendingMu.Unlock()
		return err
	}

	select {
	case env := <-responseCh:
		if env.Error != nil {
			return fmt.Errorf("%s: %s (%d)", method, env.Error.Message, env.Error.Code)
		}
		if out == nil || len(env.Result) == 0 || string(env.Result) == "null" {
			return nil
		}
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("%s: decode response: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, key)
		c.pendingMu.Unlock()
		return fmt.Errorf("%s: %w", method, ctx.Err())
	case <-c.closed:
		if err := c.lastExitError(); err != nil {
			return fmt.Errorf("%s: api client exited: %w", method, err)
		}
		return fmt.Errorf("%s: api client closed", method)
	}
}

func (c *Client) write(payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return err
	}
	_, err = c.stdin.Write(body)
	return err
}

func (c *Client) readLoop() {
	for {
		env, err := readEnvelope(c.reader)
		if err != nil {
			if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "file already closed") {
				return
			}
			select {
			case <-c.closed:
				return
			default:
				fmt.Fprintf(os.Stderr, "api read error: %v\n%s\n", err, c.stderr.String())
				return
			}
		}

		if len(env.ID) > 0 && env.Method == "" {
			key := rawID(env.ID)
			c.pendingMu.Lock()
			ch, ok := c.pending[key]
			if ok {
				delete(c.pending, key)
			}
			c.pendingMu.Unlock()
			if ok {
				ch <- env
			}
		}
	}
}

func (c *Client) setExitError(err error) {
	c.exitErrMu.Lock()
	defer c.exitErrMu.Unlock()
	c.exitErr = err
}

func (c *Client) lastExitError() error {
	c.exitErrMu.Lock()
	defer c.exitErrMu.Unlock()
	return c.exitErr
}

func readEnvelope(reader *bufio.Reader) (rpcEnvelope, error) {
	var contentLength int
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return rpcEnvelope{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			_, err := fmt.Sscanf(line, "Content-Length: %d", &contentLength)
			if err != nil {
				_, err = fmt.Sscanf(line, "content-length: %d", &contentLength)
			}
			if err != nil {
				return rpcEnvelope{}, err
			}
		}
	}

	if contentLength <= 0 {
		return rpcEnvelope{}, errors.New("missing content length")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return rpcEnvelope{}, err
	}

	var env rpcEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return rpcEnvelope{}, err
	}
	return env, nil
}

func rawID(id json.RawMessage) string {
	return strings.Trim(string(id), "\"")
}
