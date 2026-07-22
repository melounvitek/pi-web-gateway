package server

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/melounvitek/gripi/internal/config"
	"github.com/melounvitek/gripi/internal/rendering"
	"github.com/melounvitek/gripi/internal/rpc"
	"github.com/melounvitek/gripi/internal/sessions"
)

func TestCanonicalRPCSessionPathMovesPendingClientAndGatewayAttachments(t *testing.T) {
	root := t.TempDir()
	sessionsRoot := filepath.Join(root, "sessions")
	attachmentsRoot := filepath.Join(root, "attachments")
	project := filepath.Join(root, "project")
	for _, path := range []string{sessionsRoot, attachmentsRoot, project} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	realPath := filepath.Join(sessionsRoot, "real.jsonl")
	header := map[string]any{"type": "session", "version": 3, "id": "real", "cwd": project}
	encoded, _ := json.Marshal(header)
	if err := os.WriteFile(realPath, append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	pendingPath := filepath.Join(sessionsRoot, "pending.jsonl")
	client := &remapClient{state: map[string]any{"type": "response", "success": true, "data": map[string]any{"sessionFile": realPath}}}
	registry := rpc.NewRegistry(func(string) (rpc.RPCClient, error) { return nil, os.ErrNotExist }, nil)
	if err := registry.Register(pendingPath, client); err != nil {
		t.Fatal(err)
	}
	pending := rpc.NewPendingSessionRegistry(nil)
	pending.Remember(pendingPath, project)
	metadata := filepath.Join(attachmentsRoot, sessions.SessionHash(pendingPath)+".jsonl")
	if err := os.WriteFile(metadata, []byte("gateway metadata\n"), 0600); err != nil {
		t.Fatal(err)
	}
	app := &application{config: config.Config{SessionsRoot: sessionsRoot, AttachmentsRoot: attachmentsRoot}, sessionCache: sessions.NewCache(), rpcClients: registry, pendingSessions: pending}

	result, err := app.canonicalRPCSessionPath(context.Background(), pendingPath)
	if err != nil || result != realPath {
		t.Fatalf("remapped path = %q, %v", result, err)
	}
	if registry.Active(pendingPath) || !registry.Active(realPath) {
		t.Fatal("client was not moved atomically")
	}
	if _, ok := pending.CWD(pendingPath); ok {
		t.Fatal("pending metadata remained")
	}
	if remapped, ok := pending.Resolve(pendingPath); !ok || remapped != realPath {
		t.Fatalf("pending remap alias = %q, %v", remapped, ok)
	}
	migrated, err := os.ReadFile(filepath.Join(attachmentsRoot, sessions.SessionHash(realPath)+".jsonl"))
	if err != nil || string(migrated) != "gateway metadata\n" {
		t.Fatalf("migrated metadata = %q, %v", migrated, err)
	}
}

func TestEventsContinuesResolvingACompletedPendingSessionRemap(t *testing.T) {
	client := &remapClient{}
	registry := rpc.NewRegistry(func(string) (rpc.RPCClient, error) { return nil, os.ErrNotExist }, nil)
	if err := registry.Register("/real", client); err != nil {
		t.Fatal(err)
	}
	pending := rpc.NewPendingSessionRegistry(nil)
	pending.Remap("/pending", "/real")
	app := &application{config: config.Config{}, sessionCache: sessions.NewCache(), rpcClients: registry, pendingSessions: pending}
	for iteration := 0; iteration < 2; iteration++ {
		request := httptest.NewRequest(http.MethodGet, "http://app.test/events?session="+url.QueryEscape("/pending")+"&after=0", nil)
		response := httptest.NewRecorder()
		app.events(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"session_sync":{"error":null,"gateway_busy":false,"mode":"available"`) {
			t.Fatalf("event poll %d = %d %s", iteration, response.Code, response.Body.String())
		}
	}
}

func TestPreparePageUsesManagedRPCLeafAndLiveSnapshot(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "session.jsonl")
	records := []map[string]any{
		{"type": "session", "version": 3, "id": "session", "timestamp": "2026-01-01T00:00:00Z", "cwd": project},
		{"type": "message", "id": "branch-a", "parentId": nil, "timestamp": "2026-01-01T00:00:01Z", "message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "Branch A"}}}},
		{"type": "message", "id": "source", "parentId": "branch-a", "timestamp": "2026-01-01T00:00:02Z", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "toolCall", "id": "tool-1", "name": "subagent", "arguments": map[string]any{"task": "Review active work"}}}}},
		{"type": "message", "id": "branch-b", "parentId": nil, "timestamp": "2026-01-01T00:00:03Z", "message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "Branch B"}}}},
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	client := &remapClient{position: rpc.SessionEntries{Known: true, LeafID: "source"}, live: rpc.LiveSnapshot{EventSequence: 9, ActiveToolEvents: []map[string]any{{"type": "tool_execution_update", "toolCallId": "tool-1", "toolName": "subagent"}}, AgentRunning: true}}
	registry := rpc.NewRegistry(func(string) (rpc.RPCClient, error) { return client, nil }, nil)
	if err := registry.Register(path, client); err != nil {
		t.Fatal(err)
	}
	cache := sessions.NewCache()
	app := &application{config: config.Config{SessionsRoot: root, Home: root, AttachmentsRoot: filepath.Join(root, "attachments")}, sessionCache: cache, gatewayState: sessions.NewGatewayState(filepath.Join(root, "read.json"), filepath.Join(root, "pinned.json")), rpcClients: registry, pendingSessions: rpc.NewPendingSessionRegistry(nil), instanceID: "test"}
	app.synchronizer = sessions.NewSynchronizer(root, root, cache, registry)
	request := httptest.NewRequest(http.MethodGet, "http://app.test/?session="+url.QueryEscape(path), nil)
	view, err := app.preparePage(request, true)
	if err != nil {
		t.Fatal(err)
	}
	if view.Window.TreeLeafID != "source" || len(view.Window.Messages) < 1 || view.Window.Messages[0].Text != "Branch A" {
		t.Fatalf("managed window = %#v", view.Window)
	}
	if view.LiveOutput.EventAfter != 9 || view.LiveOutput.ComposerState != "running" || !strings.Contains(view.LiveOutput.ActiveToolEventsJSON, "tool-1") || !strings.Contains(view.LiveOutput.ActiveToolPromptsJSON, "Review active work") || !strings.Contains(view.LiveOutput.ActiveToolTimestampsJSON, "tool-1") {
		t.Fatalf("live output = %#v", view.LiveOutput)
	}
	if view.SessionSyncMode != sessions.SyncManaged {
		t.Fatalf("sync mode = %s", view.SessionSyncMode)
	}
	app.templates, err = template.New("").Funcs(templateFunctions(rendering.NewMarkdown())).ParseFS(templateFiles, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var rendered strings.Builder
	if err := app.templates.ExecuteTemplate(&rendered, "conversation", view); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`data-events-after="9"`, `tool-1`, `data-composer-state="running"`, `data-session-sync-mode="managed"`} {
		if !strings.Contains(rendered.String(), expected) {
			t.Errorf("rendered conversation does not contain %q", expected)
		}
	}
}

func TestPreparePageExposesExternalFollowAndUsesPersistedLeaf(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "session.jsonl")
	writeSessionRecords(t, path, []map[string]any{{"type": "session", "version": 3, "id": "session", "timestamp": "2026-01-01T00:00:00Z", "cwd": project}, {"type": "message", "id": "old", "parentId": nil, "timestamp": "2026-01-01T00:00:01Z", "message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "Old"}}}}})
	registry := rpc.NewRegistry(func(string) (rpc.RPCClient, error) { return nil, os.ErrNotExist }, nil)
	cache := sessions.NewCache()
	app := &application{config: config.Config{SessionsRoot: root, Home: root, AttachmentsRoot: filepath.Join(root, "attachments")}, sessionCache: cache, gatewayState: sessions.NewGatewayState(filepath.Join(root, "read.json"), filepath.Join(root, "pinned.json")), rpcClients: registry, pendingSessions: rpc.NewPendingSessionRegistry(nil), instanceID: "test"}
	app.synchronizer = sessions.NewSynchronizer(root, root, cache, registry)
	request := httptest.NewRequest(http.MethodGet, "http://app.test/?session="+url.QueryEscape(path), nil)
	if _, err := app.preparePage(request, true); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewEncoder(file).Encode(map[string]any{"type": "message", "id": "external", "parentId": "old", "timestamp": "2026-01-01T00:00:02Z", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "External"}}}})
	_ = file.Close()
	view, err := app.preparePage(request, true)
	if err != nil {
		t.Fatal(err)
	}
	if !view.SessionSyncBlocked || view.SessionSyncMode != sessions.SyncExternalFollow || view.Window.TreeLeafID != "external" {
		t.Fatalf("external view: blocked=%v mode=%s leaf=%s", view.SessionSyncBlocked, view.SessionSyncMode, view.Window.TreeLeafID)
	}
}

func TestLiveOutputRecognizesNativeIntegerCompletedBashAsPersisted(t *testing.T) {
	recorded := time.UnixMilli(1_750_000_000_500)
	exitCode := 0
	messages := []*sessions.Message{{Role: "bashExecution", Summary: "$ printf done", Text: "done", BashExitCode: &exitCode, BashRecordedAt: recorded}}
	snapshot := rpc.LiveSnapshot{CompletedBashEvents: []map[string]any{{"type": "bash_end", "bashId": "bash-1", "command": "printf done", "startedAt": int64(1_750_000_000_000), "result": map[string]any{"output": "done", "exitCode": int64(0)}}}}
	output := liveOutputFrom(snapshot, messages, "/home/test", nil)
	if output.CompletedBashEventsJSON != "[]" || output.PersistedBashJSON != `["bash-1"]` {
		t.Fatalf("completed=%s persisted=%s", output.CompletedBashEventsJSON, output.PersistedBashJSON)
	}
}

func TestHandlerCloseHonorsContextWhileMaintenanceFinishes(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	maintenance, err := rpc.NewMaintenance(time.Millisecond, func(context.Context) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	maintenance.Start(context.Background())
	<-started
	handler := &Handler{app: &application{rpcClients: rpc.NewRegistry(func(string) (rpc.RPCClient, error) { return nil, os.ErrNotExist }, nil), rpcMaintenance: maintenance}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := handler.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("close error = %v", err)
	}
	close(release)
	if err := handler.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPendingClientMoveRollsBackWhenAttachmentMigrationFails(t *testing.T) {
	root := t.TempDir()
	from, to := "/pending", "/real"
	client := &remapClient{}
	registry := rpc.NewRegistry(func(string) (rpc.RPCClient, error) { return nil, os.ErrNotExist }, nil)
	if err := registry.Register(from, client); err != nil {
		t.Fatal(err)
	}
	pending := rpc.NewPendingSessionRegistry(nil)
	pending.Remember(from, "/project")
	source := filepath.Join(root, sessions.SessionHash(from)+".jsonl")
	if err := os.WriteFile(source, []byte("metadata\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, sessions.SessionHash(to)+".jsonl"), 0700); err != nil {
		t.Fatal(err)
	}
	app := &application{config: config.Config{AttachmentsRoot: root}, rpcClients: registry, pendingSessions: pending}
	if err := app.movePendingRPCClient(from, to); err == nil {
		t.Fatal("attachment migration unexpectedly succeeded")
	}
	if !registry.Active(from) || registry.Active(to) {
		t.Fatal("failed migration did not roll back client move")
	}
	if _, ok := pending.CWD(from); !ok {
		t.Fatal("failed migration forgot pending metadata")
	}
}

func writeSessionRecords(t *testing.T, path string, records []map[string]any) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

type remapClient struct {
	state    map[string]any
	position rpc.SessionEntries
	live     rpc.LiveSnapshot
}

func (client *remapClient) Close() error             { return nil }
func (client *remapClient) Busy() bool               { return false }
func (client *remapClient) BusySince() *time.Time    { return nil }
func (client *remapClient) SettledAt() *time.Time    { return nil }
func (client *remapClient) AgentRunning() bool       { return false }
func (client *remapClient) Compacting() bool         { return false }
func (client *remapClient) EventSequence() int64     { return 0 }
func (client *remapClient) EventReplayCursor() int64 { return 0 }
func (client *remapClient) EventsAfter(int64) rpc.EventBatch {
	return rpc.EventBatch{Events: []map[string]any{}}
}
func (client *remapClient) LiveSnapshot() rpc.LiveSnapshot {
	if client.live.ActiveToolEvents == nil {
		client.live.ActiveToolEvents = []map[string]any{}
	}
	return client.live
}
func (client *remapClient) GetState(context.Context) (map[string]any, error) {
	return client.state, nil
}
func (client *remapClient) GetSessionStats(context.Context) (map[string]any, error) { return nil, nil }
func (client *remapClient) GetCommands(context.Context) (map[string]any, error)     { return nil, nil }
func (client *remapClient) SessionPosition(context.Context, string) (rpc.SessionEntries, error) {
	if !client.position.Known && client.position.Error == "" {
		return rpc.SessionEntries{Known: true}, nil
	}
	return client.position, nil
}
func (client *remapClient) SessionEntriesAfter(context.Context, string) (rpc.SessionEntries, error) {
	return client.SessionPosition(context.Background(), "")
}
