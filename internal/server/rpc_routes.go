package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/melounvitek/gripi/internal/rpc"
	"github.com/melounvitek/gripi/internal/sessions"
)

func (app *application) events(response http.ResponseWriter, request *http.Request) {
	path, err := app.canonicalCommandSessionPath(request.Context(), request.URL.Query().Get("session"))
	if err != nil {
		http.Error(response, "Unable to remap pending session", http.StatusInternalServerError)
		return
	}
	if path == "" {
		http.NotFound(response, request)
		return
	}
	after, _ := strconv.ParseInt(request.URL.Query().Get("after"), 10, 64)
	batch := app.rpcClients.EventsAfter(path, after)
	payload := map[string]any{"events": batch.Events, "last_seq": batch.LastSeq, "missed": batch.Missed}
	if _, err := os.Stat(path); err == nil {
		state := app.synchronizer.InspectIfAvailable(request.Context(), path, false)
		if state != nil {
			payload["session_sync"] = map[string]any{"mode": state.Mode, "revision": nullableString(state.Revision), "error": nullableString(state.Error), "gateway_busy": app.rpcClients.Busy(path)}
		}
	} else {
		payload["session_sync"] = map[string]any{"mode": sessions.SyncAvailable, "revision": nil, "error": nil, "gateway_busy": app.rpcClients.Busy(path)}
	}
	writeJSON(response, payload)
}

func (app *application) liveSessionStatus(response http.ResponseWriter, request *http.Request) {
	path, err := app.canonicalCommandSessionPath(request.Context(), request.URL.Query().Get("session"))
	if err != nil {
		http.Error(response, "Unable to remap pending session", http.StatusInternalServerError)
		return
	}
	if path == "" {
		http.NotFound(response, request)
		return
	}
	live := app.readLiveStatus(request.Context(), path)
	var status sessions.Status
	if live == nil || !live.diskIndependent {
		if _, err := os.Stat(path); err == nil {
			store := sessions.Store{Root: app.config.SessionsRoot, Home: app.config.Home, Cache: app.sessionCache}
			persisted, err := store.Status(path)
			if err != nil {
				http.NotFound(response, request)
				return
			}
			status = persisted
		}
	}
	if live != nil {
		applyLiveStatus(&status, live)
	}
	contextUsage, model := "", ""
	for _, item := range statusItems(status) {
		switch item[0] {
		case "CTX":
			contextUsage = item[1]
		case "Model":
			model = strings.TrimSuffix(item[1], " ("+status.ThinkingLevel+")")
		}
	}
	writeJSON(response, map[string]any{"context": contextUsage, "model": model, "thinking": status.ThinkingLevel})
}

func (app *application) commands(response http.ResponseWriter, request *http.Request) {
	path, err := app.canonicalCommandSessionPath(request.Context(), request.URL.Query().Get("session"))
	if err != nil {
		http.Error(response, "Unable to remap pending session", http.StatusInternalServerError)
		return
	}
	if path == "" {
		http.NotFound(response, request)
		return
	}
	app.rpcDiagnostics.Log("request_operation", map[string]any{"path": request.URL.Path, "session": path, "lane": "operation"})
	var rpcResponse map[string]any
	err = app.withSynchronizedClient(request.Context(), path, func(client rpc.RPCClient) error {
		var requestErr error
		rpcResponse, requestErr = client.GetCommands(request.Context())
		return requestErr
	})
	commands := rpc.CommandsFrom(rpcResponse)
	if err != nil {
		if app.writeRPCError(response, err) {
			return
		}
		commands = rpc.BuiltinCommands()
	}
	groups := make([]commandGroup, 0, 4)
	for _, source := range []string{"extension", "prompt", "skill", "other"} {
		group := commandGroup{Source: source}
		for _, command := range commands {
			commandSource := stringFromAny(command["source"])
			if commandSource == "" {
				commandSource = "other"
			}
			if commandSource == source {
				group.Commands = append(group.Commands, commandView{Name: stringFromAny(command["name"]), Description: stringFromAny(command["description"])})
			}
		}
		if len(group.Commands) > 0 {
			groups = append(groups, group)
		}
	}
	response.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = app.templates.ExecuteTemplate(response, "commands_list", struct {
		Count  int
		Groups []commandGroup
	}{len(commands), groups})
}

type commandGroup struct {
	Source   string
	Commands []commandView
}
type commandView struct{ Name, Description string }

func (command commandView) Text() string { return "/" + command.Name + " " + command.Description }

type liveStatus struct {
	provider, modelID, thinking string
	context                     map[string]any
	diskIndependent             bool
}

func (app *application) readLiveStatus(ctx context.Context, path string) *liveStatus {
	var stateResponse, statsResponse map[string]any
	err := app.rpcClients.WithObservingClient(ctx, path, false, func(client rpc.RPCClient) error {
		var err error
		stateResponse, err = client.GetState(ctx)
		if err != nil {
			return err
		}
		statsResponse, err = client.GetSessionStats(ctx)
		return err
	})
	if err != nil || stateResponse == nil || statsResponse == nil {
		return nil
	}
	state := successfulData(stateResponse)
	stats := successfulData(statsResponse)
	result := &liveStatus{}
	if model, ok := state["model"].(map[string]any); ok {
		result.provider = nonemptyString(model["provider"])
		result.modelID = nonemptyString(model["id"])
	}
	result.thinking = nonemptyString(state["thinkingLevel"])
	if contextUsage, ok := stats["contextUsage"].(map[string]any); ok && validContextUsage(contextUsage) {
		result.context = contextUsage
	}
	result.diskIndependent = result.provider != "" && result.modelID != "" && result.thinking != "" && result.context != nil
	return result
}

func applyLiveStatus(status *sessions.Status, live *liveStatus) {
	if live.provider != "" {
		status.Provider = live.provider
	}
	if live.modelID != "" {
		status.ModelID = live.modelID
	}
	if live.thinking != "" {
		status.ThinkingLevel = live.thinking
	}
	if live.context != nil {
		if value, ok := numericValue(live.context["tokens"]); ok {
			status.ContextTokens = value
			status.HasContextTokens = true
		} else {
			status.ContextTokens = 0
			status.HasContextTokens = false
		}
		if value, ok := numericValue(live.context["contextWindow"]); ok {
			status.ContextLimit = value
			status.HasContextLimit = true
		} else {
			status.ContextLimit = 0
			status.HasContextLimit = false
		}
		if value, ok := numericValue(live.context["percent"]); ok {
			status.ContextPercent = value
		} else {
			status.ContextPercent = 0
		}
		status.ContextEstimated = false
	}
}

func (app *application) withSynchronizedClient(ctx context.Context, path string, call func(rpc.RPCClient) error) error {
	if _, err := os.Stat(path); err == nil {
		return app.synchronizer.WithMutableClient(ctx, path, call)
	}
	return app.rpcClients.WithClient(ctx, path, call)
}

func (app *application) writeRPCError(response http.ResponseWriter, err error) bool {
	var blocked *sessions.SyncBlockedError
	var timeout *rpc.RequestTimeoutError
	switch {
	case errors.As(err, &blocked):
		writeJSONStatus(response, http.StatusConflict, map[string]any{"error": blocked.Error(), "session_sync_mode": blocked.Mode})
		return true
	case errors.Is(err, sessions.ErrSyncBusy), errors.Is(err, rpc.ErrOperationPending):
		writeJSONStatus(response, http.StatusConflict, map[string]any{"code": "session_operation_pending", "error": "Another session operation is pending. Please retry."})
		return true
	case errors.Is(err, rpc.ErrClientRetiring):
		response.Header().Set("Retry-After", "1")
		writeJSONStatus(response, http.StatusServiceUnavailable, map[string]any{"error": "Pi RPC client is restarting"})
		return true
	case errors.As(err, &timeout):
		writeJSONStatus(response, http.StatusGatewayTimeout, map[string]any{"error": err.Error()})
		return true
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return true
	default:
		return false
	}
}

func (app *application) canonicalCommandSessionPath(ctx context.Context, path string) (string, error) {
	if path == "" {
		return "", nil
	}
	canonical, err := app.canonicalRPCSessionPath(ctx, path)
	if err != nil || !app.commandSessionAvailable(canonical) {
		return "", err
	}
	return canonical, nil
}

func (app *application) commandSessionAvailable(path string) bool {
	if path == "" {
		return false
	}
	if app.rpcClients.Active(path) {
		return true
	}
	store := sessions.Store{Root: app.config.SessionsRoot, Home: app.config.Home, Cache: app.sessionCache}
	_, ok := store.Session(path)
	return ok
}

func (app *application) canonicalRPCSessionPath(ctx context.Context, path string) (string, error) {
	if remapped, ok := app.pendingSessions.Resolve(path); ok {
		return remapped, nil
	}
	if cwd, ok := app.pendingSessions.CWD(path); ok && app.rpcClients.Active(path) {
		var state map[string]any
		if app.rpcClients.WithExistingClient(ctx, path, true, func(client rpc.RPCClient) error { var err error; state, err = client.GetState(ctx); return err }) == nil {
			real := sessionFileFrom(state)
			if real != "" {
				store := sessions.Store{Root: app.config.SessionsRoot, Home: app.config.Home, Cache: app.sessionCache}
				if session, ok := store.Session(real); ok && session.CWD == cwd {
					if err := app.movePendingRPCClient(path, real); err != nil {
						return path, err
					}
					return real, nil
				}
			}
		}
		return path, nil
	}
	if app.rpcClients.Active(path) {
		return path, nil
	}
	store := sessions.Store{Root: app.config.SessionsRoot, Home: app.config.Home, Cache: app.sessionCache}
	session, known := store.Session(path)
	if !known {
		return path, nil
	}
	for _, pending := range app.pendingSessions.Entries() {
		if pending.CWD != session.CWD || !app.rpcClients.Active(pending.Path) {
			continue
		}
		var state map[string]any
		err := app.rpcClients.WithExistingClient(ctx, pending.Path, true, func(client rpc.RPCClient) error {
			var requestErr error
			state, requestErr = client.GetState(ctx)
			return requestErr
		})
		if err == nil && sessionFileFrom(state) == path {
			if err := app.movePendingRPCClient(pending.Path, path); err != nil {
				return path, err
			}
			break
		}
	}
	return path, nil
}

func (app *application) movePendingRPCClient(from, to string) error {
	err := app.rpcClients.MoveWith(from, to, func() (func() error, error) {
		return (sessions.AttachmentStore{Root: app.config.AttachmentsRoot}).Migrate(from, to)
	})
	if err != nil {
		return err
	}
	app.pendingSessions.Remap(from, to)
	return nil
}

func (app *application) cleanupIdleRPCClients(ctx context.Context) error {
	if app.config.RPCIdleTimeout <= 0 {
		return nil
	}
	now := time.Now()
	paths := app.rpcClients.IdleClientPaths(app.config.RPCIdleTimeout, now, nil)
	var first error
	for _, path := range paths {
		if _, pending := app.pendingSessions.CWD(path); pending {
			continue
		}
		var closed bool
		var err error
		if _, statErr := os.Stat(path); statErr == nil {
			app.synchronizer.ReconcileIfAvailable(ctx, path, false, func(sessions.SyncResult) {
				closed, err = app.rpcClients.CloseClientIfExpired(path, app.config.RPCIdleTimeout, now, nil)
				if closed {
					app.synchronizer.Forget(path)
				}
			})
		} else {
			closed, err = app.rpcClients.CloseClientIfExpired(path, app.config.RPCIdleTimeout, now, nil)
		}
		if err != nil && first == nil {
			first = err
		}
		if closed {
			app.pendingSessions.Forget(path)
		}
	}
	return first
}

func successfulData(response map[string]any) map[string]any {
	if response["success"] != true {
		return nil
	}
	data, _ := response["data"].(map[string]any)
	return data
}
func sessionFileFrom(response map[string]any) string {
	data := response
	if nested, ok := response["data"].(map[string]any); ok {
		data = nested
	}
	for _, key := range []string{"sessionFile", "session_file", "path"} {
		if value := stringFromAny(data[key]); value != "" {
			return value
		}
	}
	return ""
}
func validContextUsage(value map[string]any) bool {
	for _, key := range []string{"tokens", "contextWindow", "percent"} {
		if value[key] == nil {
			continue
		}
		if _, ok := numericValue(value[key]); !ok {
			return false
		}
	}
	return true
}
func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		result, err := typed.Float64()
		return result, err == nil
	default:
		return 0, false
	}
}
func stringFromAny(value any) string  { result, _ := value.(string); return result }
func nonemptyString(value any) string { return stringFromAny(value) }
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func writeJSONStatus(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	writeJSON(response, value)
}
