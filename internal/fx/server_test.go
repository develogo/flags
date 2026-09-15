package fx_test

import (
	"encoding/json"
	"flags/internal/config"
	fxserver "flags/internal/fx"
	"flags/internal/handlers"
	"flags/internal/middleware"
	"flags/internal/services"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
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
	Key     string // API key recebida, que seleciona o flag set
	Flag    string
	Header  http.Header
	Context struct {
		Key    string         `json:"key"`
		Custom map[string]any `json:"custom"`
	}
}

// fakeRelay imita o endpoint de avaliação do relay GOFF
// (POST /v1/feature/<flag>/eval) com um flag set por app: a API key
// (Authorization: Bearer <app>) escolhe o flag set. Registra cada requisição.
type fakeRelay struct {
	*httptest.Server

	mu       sync.Mutex
	flagSets map[string]map[string]any // app -> flag -> valor
	down     bool
	requests []relayRequest
}

// publicApps são os apps públicos de fixture; backend fica privado.
var publicApps = []string{"bettercity-flutter", "other"}

// flutterFlagSet são os valores do relay para bettercity-flutter.
var flutterFlagSet = map[string]map[string]any{
	"bettercity-flutter": {"dark_mode": true, "app_version": "2.0.0"},
}

// O relay fake e o evaluator são únicos no pacote. Ao registrar um provider, o
// OpenFeature SDK compara por reflexão os providers já ativos, inclusive o
// http.Transport deles; criar providers a cada teste disputaria com as conexões
// de testes anteriores. Em produção os providers são criados uma vez no boot.
var (
	sharedRelay *fakeRelay
	evaluator   *services.FeatureFlagService
)

func TestMain(m *testing.M) {
	sharedRelay = &fakeRelay{}
	sharedRelay.Server = httptest.NewServer(http.HandlerFunc(sharedRelay.serveEval))

	cfg := &config.Config{Goff: config.GoffConfig{Endpoint: sharedRelay.URL}}
	var err error
	evaluator, err = services.NewFeatureFlagService(cfg, publicApps, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		panic(err)
	}

	code := m.Run()
	sharedRelay.Close()
	os.Exit(code)
}

// newFakeRelay prepara o relay fake do pacote para o teste: flag sets dados,
// relay no ar e nenhuma requisição registrada.
func resetRelay(t *testing.T, flagSets map[string]map[string]any) *fakeRelay {
	t.Helper()
	sharedRelay.mu.Lock()
	defer sharedRelay.mu.Unlock()
	sharedRelay.flagSets = flagSets
	sharedRelay.down = false
	sharedRelay.requests = nil
	return sharedRelay
}

// GoDown faz o relay derrubar toda conexão sem responder, como um relay fora do ar.
func (r *fakeRelay) GoDown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.down = true
}

func (r *fakeRelay) serveEval(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	down := r.down
	r.mu.Unlock()
	if down {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
		return
	}

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
	key, _ := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	recorded := relayRequest{Key: key, Flag: flag, Header: req.Header.Clone()}
	if err := json.Unmarshal(data, &body); err != nil || json.Unmarshal(body.EvaluationContext, &recorded.Context) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	r.mu.Lock()
	r.requests = append(r.requests, recorded)
	flagSet, knownKey := r.flagSets[key]
	value, found := flagSet[flag]
	r.mu.Unlock()

	if !knownKey {
		http.Error(w, `{"message":"invalid key"}`, http.StatusUnauthorized)
		return
	}

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
// sobre o registry de fixtures e o evaluator do pacote.
func newTestServer(t *testing.T) *echo.Echo {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{App: config.AppConfig{Port: "0", RateLimit: 100}}

	registry, err := services.NewFlagRegistryService(testAppsDir, publicApps, logger)
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

// Rota legada: sem app e com app=flutter, atende bettercity-flutter.
func TestFlags_LegacyRouteServesBetterCityFlutter(t *testing.T) {
	for _, target := range []string{"/api/v1/flags?app=flutter", "/api/v1/flags", "/api/v1/flags?app=bettercity-flutter"} {
		t.Run(target, func(t *testing.T) {
			relay := resetRelay(t, flutterFlagSet)
			e := newTestServer(t)

			rec := get(e, target, map[string]string{"Device-ID": "device-1", "Platform": "android", "App-Version": "3.1.0"})

			require.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
			assert.Equal(t, map[string]any{"dark_mode": true, "app_version": "2.0.0"}, decodeFlags(t, rec))

			requests := relay.Requests()
			flags := make([]string, 0, len(requests))
			for _, r := range requests {
				flags = append(flags, r.Flag)
				assert.Equal(t, "bettercity-flutter", r.Key)
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
	relay := resetRelay(t, flutterFlagSet)
	e := newTestServer(t)

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
	relay := resetRelay(t, flutterFlagSet)
	e := newTestServer(t)

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
	resetRelay(t, flutterFlagSet)
	e := newTestServer(t)
	headers := map[string]string{"Device-ID": "device-1", "Platform": "android", "App-Version": "3.1.0"}

	plain := get(e, "/api/v1/flags?app=flutter", headers)
	headers["Authorization"] = "Bearer lixo"
	withToken := get(e, "/api/v1/flags?app=flutter", headers)

	require.Equal(t, http.StatusOK, plain.Code)
	assert.Equal(t, plain.Code, withToken.Code)
	assert.JSONEq(t, plain.Body.String(), withToken.Body.String())
}

func TestFlags_SameFlagInTwoAppsReturnsEachAppValue(t *testing.T) {
	resetRelay(t, map[string]map[string]any{
		"bettercity-flutter": {"dark_mode": true, "app_version": "2.0.0"},
		"other":              {"dark_mode": false},
	})
	e := newTestServer(t)

	flutter := get(e, "/api/v1/flags?app=bettercity-flutter", nil)
	other := get(e, "/api/v1/flags?app=other", nil)

	require.Equal(t, http.StatusOK, flutter.Code)
	require.Equal(t, http.StatusOK, other.Code)
	assert.Equal(t, true, decodeFlags(t, flutter)["dark_mode"])
	assert.Equal(t, map[string]any{"dark_mode": false}, decodeFlags(t, other))
}

func TestFlags_ConcurrentRequestsForDifferentApps(t *testing.T) {
	resetRelay(t, map[string]map[string]any{
		"bettercity-flutter": {"dark_mode": true, "app_version": "2.0.0"},
		"other":              {"dark_mode": false},
	})
	e := newTestServer(t)
	want := map[string]bool{"bettercity-flutter": true, "other": false}

	type result struct {
		app string
		rec *httptest.ResponseRecorder
	}
	results := make(chan result, 20*len(want))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		for app := range want {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- result{app, get(e, "/api/v1/flags?app="+app, nil)}
			}()
		}
	}
	wg.Wait()
	close(results)

	for r := range results {
		require.Equal(t, http.StatusOK, r.rec.Code, r.app)
		assert.Equal(t, want[r.app], decodeFlags(t, r.rec)["dark_mode"], r.app)
	}
}

// App privado e app desconhecido recebem a mesma resposta, sem chamar o relay.
func TestFlags_PrivateAndUnknownAppsAreIndistinguishable(t *testing.T) {
	relay := resetRelay(t, map[string]map[string]any{"backend": {"cache_enabled": false}})
	e := newTestServer(t)

	for _, app := range []string{"backend", "nonexistent"} {
		rec := get(e, "/api/v1/flags?app="+app, nil)

		assert.Equal(t, http.StatusBadRequest, rec.Code, app)
		assert.JSONEq(t, `{"error":"Unknown application: `+app+`"}`, rec.Body.String(), app)
	}
	assert.Empty(t, relay.Requests())
}

func TestFlags_RelayDownReturnsFileDefaults(t *testing.T) {
	relay := resetRelay(t, flutterFlagSet)
	e := newTestServer(t)
	relay.GoDown()

	rec := get(e, "/api/v1/flags?app=flutter", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, map[string]any{"dark_mode": false, "app_version": "1.0.0"}, decodeFlags(t, rec))
}

func TestHealth_AlwaysOK(t *testing.T) {
	relay := resetRelay(t, flutterFlagSet)
	e := newTestServer(t)
	relay.GoDown()

	rec := get(e, "/health", nil)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReady_PassesWithHealthyRelay(t *testing.T) {
	relay := resetRelay(t, flutterFlagSet)
	e := newTestServer(t)

	rec := get(e, "/ready", nil)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ready"}`, rec.Body.String())
	requests := relay.Requests()
	require.NotEmpty(t, requests)
	assert.Equal(t, "bettercity-flutter", requests[0].Key)
}

func TestReady_FailsWithBrokenRelay(t *testing.T) {
	relay := resetRelay(t, flutterFlagSet)
	e := newTestServer(t)
	relay.GoDown()

	rec := get(e, "/ready", nil)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
