# Better Feature Flag

Serviço Go para gerenciamento de feature flags integrado com GO Feature Flag, otimizado para consumo por aplicações Flutter. Serviços backend consomem o relay proxy diretamente via SDK.

## Arquitetura

```
┌─────────────┐
│   Flutter    │ ──HTTP──> Flag API (:1324) ──SDK──> Relay Proxy (:1031)
└─────────────┘

┌─────────────┐
│  User API   │ ──SDK──────────────────────────> Relay Proxy (:1031)
└─────────────┘

┌─────────────┐
│ Payment API │ ──SDK──────────────────────────> Relay Proxy (:1031)
└─────────────┘
```

O projeto usa **interfaces** para desacoplamento total entre camadas:

```
internal/
├── config/              # Configuração via Viper (YAML + env vars + .env)
├── handlers/            # HTTP handlers (flags, health)
├── models/              # Tipos compartilhados (FlagDefinition, TokenClaims, ClientContext)
├── middleware/           # Auth (JWT opcional), CORS, rate limiting, request ID, logging
├── services/            # Interfaces + implementações (FeatureFlag, Keycloak, FlagRegistry)
│   └── interfaces.go    # FeatureFlagEvaluator, TokenValidator, FlagRegistry
└── fx/                  # Uber FX — DI, rotas, lifecycle
```

## Requisitos

- Go 1.23+
- Docker

## Como Executar

### Usando Makefile (recomendado)

```bash
make up       # Sobe relay proxy + API server (Docker)
make run      # Roda API localmente (conecta no relay em localhost:1031)
make test     # Roda testes
make logs     # Mostra logs dos containers
make down     # Para containers
make clean    # Remove tudo
```

### Manual

```bash
# 1. Instalar dependências
go mod download

# 2. Configurar secrets (criar arquivo .env na raiz)
echo "KEYCLOAK_CLIENT_SECRET=sua-chave-aqui" > .env

# 3. Subir o relay proxy
docker-compose up -d relay

# 4. Rodar a API
APP_ENV=local go run main.go server
```

## Endpoints

### Health Checks

```bash
# Liveness — aplicação está rodando
GET /health
# Response: {"status": "ok"}

# Readiness — conectividade com GOFF relay
GET /ready
# Response: {"status": "ready"} ou {"status": "unavailable", "message": "..."}
```

### Feature Flags

```bash
GET /api/v1/flags?app=flutter
```

O parâmetro `app` define qual conjunto de flags retornar (default: `flutter`).

**Headers aceitos:**

| Header | Descrição | Obrigatório |
|--------|-----------|-------------|
| `Authorization` | `Bearer <token>` — JWT do Keycloak | Opcional |
| `Device-ID` | Identificador do dispositivo | Opcional |
| `Platform` | `android` ou `ios` | Opcional |
| `Platform-Version` | Versão do SO | Opcional |
| `App-Version` | Versão do app | Opcional |
| `App-Name` | Nome do app | Opcional |
| `Device-Model` | Modelo do dispositivo | Opcional |
| `Device-Architecture` | Arquitetura da CPU | Opcional |
| `Device-Brand` | Marca do dispositivo | Opcional |
| `Mobile` | Flag de mobile | Opcional |
| `Device` | Descrição do dispositivo | Opcional |
| `Package-Name` | Nome do pacote | Opcional |
| `Build-Number` | Número do build | Opcional |

Todas as requests recebem um header `X-Request-ID` na resposta (gerado automaticamente ou propagado do request).

**Exemplos:**

Usuário autenticado:
```bash
curl http://localhost:1324/api/v1/flags?app=flutter \
  -H "Authorization: Bearer eyJhbGc..." \
  -H "Platform: android" \
  -H "App-Version: 1.2.0"
```

Usuário anônimo:
```bash
curl http://localhost:1324/api/v1/flags?app=flutter \
  -H "Device-ID: 550e8400-e29b-41d4-a716-446655440000" \
  -H "Platform: ios" \
  -H "App-Version: 1.2.0"
```

**Response:**
```json
{
  "flags": {
    "dark_mode": false,
    "maintenance_mode": false,
    "feedback_enabled": true,
    "force_update_enabled": false,
    "minimum_app_version": "1.0.0"
  }
}
```

## Configuração

### Config YAML

Carregado de `config/{APP_ENV}.yaml` (default: `local`). Variáveis de ambiente sobrescrevem valores YAML (ex: `KEYCLOAK_CLIENT_SECRET` → `keycloak.client_secret`).

Um arquivo `.env` na raiz é carregado automaticamente para desenvolvimento local.

| Campo | Descrição | Default |
|-------|-----------|---------|
| `app.port` | Porta HTTP | `1324` |
| `app.log_level` | Nível de log (`debug`, `info`, `warn`, `error`) | `info` |
| `app.cors_origins` | Lista de origens permitidas | `["*"]` |
| `app.rate_limit` | Requests por segundo por IP | `100` |
| `goff.endpoint` | URL do relay proxy | — |
| `keycloak.url` | URL do Keycloak | — |
| `keycloak.realm` | Realm do Keycloak | — |
| `keycloak.client_id` | Client ID | — |
| `keycloak.client_secret` | Client secret (usar env var) | — |

### Apps servidos

A API serve apenas os apps listados em `ServedApps` (`internal/services/registry.go`) — hoje só `flutter`. Para cada um, lê `flags/apps/<app>.yaml` no startup: o mesmo arquivo que o relay carrega. Não existe registry separado.

- **Tipo** do flag: inferido dos valores em `variations` (todos do mesmo tipo escalar: `bool`, `string`, `int` ou `float`).
- **Fallback** (usado se o relay estiver fora): o valor de `defaultRule.variation`, obrigatório.

Qualquer violação impede o servidor de subir. Ver `docs/adr/0001-goff-yaml-fonte-unica.md`.

### Flag YAML (GOFF relay)

Arquivos de flags com regras de targeting, carregados pelo relay proxy:

```
flags/
├── apps/
│   └── flutter.yaml    # Flags do app Flutter
└── shared.yaml         # Flags compartilhadas (consumidas via SDK)
```

Todas as flags usam **snake_case**.

## Adicionando Novas Flags

1. Adicione a flag no arquivo YAML do GOFF (`flags/apps/flutter.yaml`):

```yaml
nova_feature:
  variations:
    enabled: true
    disabled: false
  defaultRule:
    variation: disabled
```

2. Redeploy o serviço. A flag estará disponível no endpoint `/api/v1/flags`.

### Exemplos de Targeting

Por usuário:
```yaml
feedback_enabled:
  variations:
    enabled: true
    disabled: false
  targeting:
    - query: targetingKey eq "user-id-123"
      variation: disabled
  defaultRule:
    variation: enabled
```

Por versão do app:
```yaml
force_update_enabled:
  variations:
    enabled: true
    disabled: false
  targeting:
    - query: app_version lt "1.2.0"
      variation: enabled
  defaultRule:
    variation: disabled
```

Por plataforma:
```yaml
nova_feature:
  variations:
    enabled: true
    disabled: false
  targeting:
    - query: platform eq "android"
      variation: enabled
  defaultRule:
    variation: disabled
```

## Integração Flutter

```dart
import 'package:http/http.dart' as http;
import 'dart:convert';

class FeatureFlagService {
  final String baseUrl;

  FeatureFlagService({required this.baseUrl});

  Future<Map<String, dynamic>> getFlags({String? authToken}) async {
    final headers = <String, String>{
      'Platform': Platform.isAndroid ? 'android' : 'ios',
      'App-Version': packageInfo.version,
    };

    if (authToken != null) {
      headers['Authorization'] = 'Bearer $authToken';
    } else {
      headers['Device-ID'] = await getDeviceId();
    }

    final response = await http.get(
      Uri.parse('$baseUrl/api/v1/flags?app=flutter'),
      headers: headers,
    );

    if (response.statusCode == 200) {
      final data = json.decode(response.body);
      return data['flags'] as Map<String, dynamic>;
    } else {
      throw Exception('Failed to load flags: ${response.statusCode}');
    }
  }
}
```

## Docker

Duas imagens construídas no CI (com gate de testes):

- **Dockerfile** — Relay proxy GOFF (`gofeatureflag/go-feature-flag`), serve flag files de `flags/`
- **Dockerfile.api** — Build multi-stage Go para o API server

O `docker-compose.yml` sobe ambos localmente. A rede `bettercity_local` é compartilhada com outros serviços BetterCity.

## Testes

```bash
# Rodar todos os testes
go test ./... -v

# Com race detection (Linux/macOS)
go test ./... -race -v

# Com cobertura
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

Testes cobrem: flag registry, middleware de autenticação, middleware de request ID, handlers de flags e health.

Veja `DEPLOYMENT.md` para guia completo de deploy por ambiente.
