package fx_test

import (
	"better-feature-flag/internal/config"
	fxserver "better-feature-flag/internal/fx"
	"better-feature-flag/internal/handlers"
	"better-feature-flag/internal/middleware"
	"better-feature-flag/internal/services"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAppsDir = "../../testdata/apps"

// relayRequest é o que o relay fake recebeu numa avaliação.
type relayRequest struct {
	Flag    string
	Header  http.Header
	Context struct {
		Key    string         `json:"key"`
		Custom map[string]any `json:"custom"`
	}
}

// fakeRelay imita o endpoint de avaliação do relay GOFF
// (POST /v1/feature/<flag>/eval) e registra cada requisição recebida.
type fakeRelay struct {
	*httptest.Server

	mu       sync.Mutex
	values   map[string]any
	requests []relayRequest
}

func newFakeRelay(t *testing.T, values map[string]any) *fakeRelay {
	t.Helper()
	relay := &fakeRelay{values: values}
	relay.Server = httptest.NewServer(http.HandlerFunc(relay.serveEval))
	t.Cleanup(relay.Close)
	return relay
}

func (r *fakeRelay) serveEval(w http.ResponseWriter, req *http.Request) {
	flag, ok := strings.CutPrefix(req.URL.Path, "/v1/feature/")
	flag, ok2 := strings.CutSuffix(flag, "/eval")
	if req.Method != http.MethodPost || !ok || !ok2 {
		http.NotFound(w, req)
		return
	}

	var body struct {
		EvaluationContext json.RawMessage `json:"evaluationContext"`
	}
	data, _ := io.ReadAll(req.Body)
	recorded := relayRequest{Flag: flag, Header: req.Header.Clone()}
	if err := json.Unmarshal(data, &body); err != nil || json.Unmarshal(body.EvaluationContext, &recorded.Context) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	r.mu.Lock()
	r.requests = append(r.requests, recorded)
	value, found := r.values[flag]
	r.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if !found {
		_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": "FLAG_NOT_FOUND", "reason": "ERROR", "failed": true})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"value": value, "variationType": "relay", "reason": "TARGETING_MATCH"})
}

func (r *fakeRelay) Requests() []relayRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]relayRequest(nil), r.requests...)
}

// newTestServer monta as rotas Echo reais, com toda a pilha de middlewares,
// sobre o registry de fixtures e o evaluator real apontando para relayURL.
func newTestServer(t *testing.T, relayURL string) *echo.Echo {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		App:  config.AppConfig{Port: "0", RateLimit: 100},
		Goff: config.GoffConfig{Endpoint: relayURL},
	}

	evaluator, err := services.NewFeatureFlagService(cfg, logger)
	require.NoError(t, err)
	registry, err := services.NewFlagRegistryService(testAppsDir, services.ServedApps, logger)
	require.NoError(t, err)

	e := fxserver.ProvideEcho()
	fxserver.MountRoutes(fxserver.RouteParams{
		Echo:                    e,
		FlagsHandler:            handlers.NewFlagsHandler(evaluator, registry, logger),
		HealthHandler:           handlers.NewHealthHandler(evaluator, registry, logger),
		RateLimiter:             middleware.NewRateLimiter(cfg),
		ClientContextMiddleware: middleware.ClientContext(),
		CORSMiddleware:          middleware.CORS(cfg),
		LoggerMiddleware:        middleware.Logger(logger),
		RequestIDMiddleware:     middleware.RequestID(),
	})
	return e
}

func get(e *echo.Echo, target string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func decodeFlags(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body struct {
		Flags map[string]any `json:"flags"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Flags
}

func TestFlags_ReturnsRelayValuesForPublicApp(t *testing.T) {
	for _, target := range []string{"/api/v1/flags?app=flutter", "/api/v1/flags"} {
		t.Run(target, func(t *testing.T) {
			relay := newFakeRelay(t, map[string]any{"dark_mode": true, "app_version": "2.0.0"})
			e := newTestServer(t, relay.URL)

			rec := get(e, target, map[string]string{"Device-ID": "device-1", "Platform": "android", "App-Version": "3.1.0"})

			require.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
			assert.Equal(t, map[string]any{"dark_mode": true, "app_version": "2.0.0"}, decodeFlags(t, rec))

			requests := relay.Requests()
			flags := make([]string, 0, len(requests))
			for _, r := range requests {
				flags = append(flags, r.Flag)
				assert.Equal(t, "device-1", r.Context.Key)
				assert.Equal(t, "android", r.Context.Custom["platform"])
				assert.Equal(t, "3.1.0", r.Context.Custom["app_version"])
				assert.Equal(t, "device-1", r.Context.Custom["device_id"])
			}
			assert.ElementsMatch(t, []string{"dark_mode", "app_version"}, flags)
		})
	}
}

func TestFlags_UserIDIsTargetingKeyAndDeviceIDStaysAttribute(t *testing.T) {
	relay := newFakeRelay(t, map[string]any{"dark_mode": true, "app_version": "2.0.0"})
	e := newTestServer(t, relay.URL)

	rec := get(e, "/api/v1/flags?app=flutter", map[string]string{
		"User-ID":     "user-42",
		"Device-ID":   "device-1",
		"Platform":    "ios",
		"App-Version": "3.1.0",
	})

	require.Equal(t, http.StatusOK, rec.Code)
	requests := relay.Requests()
	require.NotEmpty(t, requests)
	for _, r := range requests {
		assert.Equal(t, "user-42", r.Context.Key)
		// O provider GOFF replica a targeting key em custom; não é atributo nosso.
		delete(r.Context.Custom, "targetingKey")
		assert.Equal(t, map[string]any{
			"user_id":     "user-42",
			"device_id":   "device-1",
			"platform":    "ios",
			"app_version": "3.1.0",
		}, r.Context.Custom)
	}
}

func TestFlags_UserIDWithoutDeviceIDOmitsDeviceAttribute(t *testing.T) {
	relay := newFakeRelay(t, map[string]any{"dark_mode": true, "app_version": "2.0.0"})
	e := newTestServer(t, relay.URL)

	rec := get(e, "/api/v1/flags?app=flutter", map[string]string{"User-ID": "user-42"})

	require.Equal(t, http.StatusOK, rec.Code)
	requests := relay.Requests()
	require.NotEmpty(t, requests)
	for _, r := range requests {
		assert.Equal(t, "user-42", r.Context.Key)
		assert.Equal(t, "user-42", r.Context.Custom["user_id"])
		assert.NotContains(t, r.Context.Custom, "device_id")
	}
}

func TestFlags_AuthorizationHeaderIsIgnored(t *testing.T) {
	relay := newFakeRelay(t, map[string]any{"dark_mode": true, "app_version": "2.0.0"})
	e := newTestServer(t, relay.URL)
	headers := map[string]string{"Device-ID": "device-1", "Platform": "android", "App-Version": "3.1.0"}

	plain := get(e, "/api/v1/flags?app=flutter", headers)
	headers["Authorization"] = "Bearer lixo"
	withToken := get(e, "/api/v1/flags?app=flutter", headers)

	require.Equal(t, http.StatusOK, plain.Code)
	assert.Equal(t, plain.Code, withToken.Code)
	assert.JSONEq(t, plain.Body.String(), withToken.Body.String())
}

func TestFlags_UnknownAppIsRejectedWithoutCallingRelay(t *testing.T) {
	relay := newFakeRelay(t, map[string]any{"dark_mode": true})
	e := newTestServer(t, relay.URL)

	rec := get(e, "/api/v1/flags?app=backend", nil)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.JSONEq(t, `{"error":"Unknown application: backend"}`, rec.Body.String())
	assert.Empty(t, relay.Requests())
}

func TestFlags_RelayDownReturnsFileDefaults(t *testing.T) {
	relay := newFakeRelay(t, map[string]any{"dark_mode": true, "app_version": "2.0.0"})
	e := newTestServer(t, relay.URL)
	relay.Close()

	rec := get(e, "/api/v1/flags?app=flutter", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, map[string]any{"dark_mode": false, "app_version": "1.0.0"}, decodeFlags(t, rec))
}

func TestHealth_AlwaysOK(t *testing.T) {
	relay := newFakeRelay(t, nil)
	e := newTestServer(t, relay.URL)
	relay.Close()

	rec := get(e, "/health", nil)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReady_PassesWithHealthyRelay(t *testing.T) {
	relay := newFakeRelay(t, map[string]any{"dark_mode": true, "app_version": "2.0.0"})
	e := newTestServer(t, relay.URL)

	rec := get(e, "/ready", nil)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ready"}`, rec.Body.String())
	assert.NotEmpty(t, relay.Requests())
}

func TestReady_FailsWithBrokenRelay(t *testing.T) {
	relay := newFakeRelay(t, map[string]any{"dark_mode": true, "app_version": "2.0.0"})
	e := newTestServer(t, relay.URL)
	relay.Close()

	rec := get(e, "/ready", nil)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
