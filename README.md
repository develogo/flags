# Flags

Serviço de feature flags compartilhado entre projetos, sobre o GO Feature Flag (GOFF). Cada **App** tem o próprio arquivo de flags e enxerga só as próprias flags. Backends na rede interna leem o relay GOFF direto via SDK OpenFeature. Apps mobile (e web) leem pela Flag API, que avalia todas as flags de um **App público** numa chamada só.

Glossário em [`CONTEXT.md`](CONTEXT.md); decisões em [`docs/adr/`](docs/adr/) (em especial a [ADR 0002](docs/adr/0002-servico-multi-app-flag-set-por-app.md)).

## Arquitetura

```
                        rede interna
                ┌───────────────────────────────────────────────┐
┌──────────┐    │                                               │
│  Mobile  │ ─HTTP─> Flag API (:1324) ─SDK (key = app)─┐        │
└──────────┘    │                                    ▼        │
                │  ┌─────────┐                 Relay GOFF      │
                │  │ Backend │ ──SDK (key = app)─> (:1031)     │
                │  └─────────┘                 um flag set     │
                │                              por app         │
                └───────────────────────────────────────────────┘
```

- O relay serve um **flag set por app**. O nome do flag set e sua única API key são o nome do app. A key seleciona o namespace, não é segredo.
- Em produção, só a Flag API fica exposta publicamente. O relay fica só na rede interna (localmente, o compose publica `:1031`).
- Não há autenticação. Quem sabe o nome de um app consegue ler as flags dele, então **flag nunca guarda segredo**.

```
internal/
├── config/        # Configuração via Viper (YAML + env vars + .env)
├── handlers/      # HTTP handlers (flags, health)
├── middleware/    # Contexto do cliente (headers), CORS, rate limiting, request ID, logging
├── models/        # Tipos compartilhados (FlagDefinition, ClientContext)
├── relayconfig/   # Gerador da config do relay (um flag set por app)
├── services/      # FeatureFlagEvaluator, FlagRegistry, ServedApps, descoberta de apps
└── fx/            # Uber FX: DI, rotas, lifecycle
```

## Requisitos

- Go 1.23+
- Docker

## Como executar

```bash
docker network create flags_local   # uma vez; o compose não cria a rede
make up       # Sobe relay (:1031) + API (:1324)
make run      # Roda a API localmente (relay em localhost:1031); rode da raiz do repo
make test     # Roda os testes
make logs     # Mostra logs dos containers
make down     # Para os containers
```

A imagem do relay é buildada com a config gerada, igual à produção. `make up` só builda quando a imagem não existe: depois de mudar flags, rode `docker-compose up -d --build`.

## Apps

Um app é um arquivo `flags/apps/<app>.yaml` no formato GOFF. O nome do arquivo (sem `.yaml`) é o nome do app. Os nomes são planos e únicos globalmente, no padrão `<projeto>-<app>`:

| App | Público | Consumido por |
|-----|---------|---------------|
| `bettercity-flutter` | sim | App Flutter, via Flag API |
| `bettercity-api` | não | Backend Go do BetterCity, via SDK no relay |

Nomes de flag podem se repetir entre apps (`maintenance_mode` existe nos dois) sem conflito.

### Adicionar um app

1. Crie `flags/apps/<projeto>-<app>.yaml`:

   ```yaml
   maintenance_mode:
     variations:
       enabled: true
       disabled: false
     defaultRule:
       variation: disabled
   ```

2. Rode `make test`. O teste `TestFlagRegistry_LoadsAllRealApps` valida o arquivo de **todos** os apps, com as mesmas regras do boot da API. No CI (push em `main`), um arquivo inválido bloqueia a publicação das imagens.
3. Faça merge e deploy. A config do relay é gerada no build da imagem (`go run . relay-config`), com um flag set para cada arquivo em `flags/apps/`. Não existe config do relay para editar à mão.

A partir daí, backends já leem o app pelo relay. Ele **não** aparece no endpoint público até ser tornado público.

Regras de todo arquivo de flags:

- Nomes de flag em **snake_case**.
- `variations` com valores de um único tipo escalar (`bool`, `string`, `int` ou `float`).
- `defaultRule.variation` obrigatório e apontando para uma variation declarada. É o valor de fallback quando o relay não responde. Rollouts percentuais ficam em `targeting`, nunca no `defaultRule`.

### Tornar um app público

Um app só é servido em `GET /api/v1/flags` se estiver em `ServedApps` (`internal/services/registry.go`):

```go
var ServedApps = []string{"bettercity-flutter", "meuprojeto-mobile"}
```

- Expor flags na internet é sempre uma decisão explícita, feita em code review.
- Kill switches e controles operacionais de backend **nunca** vão para um app público. Separe as flags do backend num app próprio (ex.: `meuprojeto-api`).
- A API recusa subir se o arquivo de um app público estiver ausente ou inválido. Apps privados não são carregados no boot, então um arquivo quebrado deles nunca derruba o endpoint.
- Pedidos para um app privado recebem a mesma resposta de um app inexistente, então o endpoint não revela os nomes dos apps privados.
- Se o app for web, adicione a origem em `app.cors_origins` (`config/production.yaml`). Mobile e backend não precisam de CORS.

## Backend: ler flags pelo relay

O backend usa o provider GOFF do OpenFeature com a **API key igual ao nome do app**. Sem a key (ou com uma key errada), o relay rejeita a requisição e o SDK devolve os defaults do código, então o backend nunca cai por causa de flags.

```go
import (
	gofeatureflag "github.com/open-feature/go-sdk-contrib/providers/go-feature-flag/pkg"
	"github.com/open-feature/go-sdk/openfeature"
)

provider, err := gofeatureflag.NewProvider(gofeatureflag.ProviderOptions{
	Endpoint: "http://relay:1031", // hostname do relay na rede interna
	APIKey:   "bettercity-api",    // nome do app = flag set
})
if err != nil {
	return err
}
if err := openfeature.SetProvider(provider); err != nil {
	return err
}
client := openfeature.NewClient("bettercity-api")

// O segundo argumento é o default do código, usado se o relay falhar.
// O provider GOFF exige targeting key: sem ela, a avaliação sempre cai no default.
evalCtx := openfeature.NewEvaluationContext("bettercity-api", nil)
enabled, _ := client.BooleanValue(ctx, "security_middlewares_enabled", true, evalCtx)
```

Backends em outros hosts não acessam o relay: em produção ele não tem endpoint público.

## Mobile: endpoint público

### Health checks

```bash
GET /health   # liveness: {"status": "ok"}
GET /ready    # readiness, avalia uma flag de um app público no relay:
              # {"status": "ready"} ou {"status": "unavailable", "message": "..."}
```

### `GET /api/v1/flags?app=<app>`

Avalia e devolve todas as flags de um app público.

- `app` precisa ser um app público. Qualquer outro nome recebe `400 {"error": "Unknown application: <app>"}`.
- **Rota legada:** `app` ausente ou `app=flutter` é atendido como `bettercity-flutter`, para os builds já instalados do Flutter BetterCity. Apps novos sempre passam `app`.
- Se o relay falhar, cada flag volta com o `defaultRule` do arquivo.
- Rate limit por IP (`app.rate_limit`).
- Toda resposta traz `X-Request-ID` (propagado do request ou gerado).

**Headers de targeting** (todos opcionais, declarados pelo cliente e não verificados):

| Header | Uso no targeting |
|--------|------------------|
| `User-ID` | Targeting key quando presente; atributo `user_id` |
| `Device-ID` | Targeting key quando não há `User-ID`; atributo `device_id` |
| `Platform` | Atributo `platform` (`android`, `ios`) |
| `App-Version` | Atributo `app_version` |

Sem `User-ID` nem `Device-ID`, a targeting key é `anonymous`. O middleware também lê `Platform-Version`, `App-Name`, `Device-Model`, `Device-Architecture`, `Device-Brand`, `Mobile`, `Device`, `Package-Name` e `Build-Number`, mas eles não entram no contexto de avaliação. `Authorization` é ignorado: um Bearer token, válido ou não, não muda o resultado.

Como qualquer cliente pode declarar qualquer `User-ID`, targeting por usuário não serve para controle de acesso.

**Exemplo:**

```bash
curl "http://localhost:1324/api/v1/flags?app=bettercity-flutter" \
  -H "User-ID: 42" \
  -H "Device-ID: 550e8400-e29b-41d4-a716-446655440000" \
  -H "Platform: android" \
  -H "App-Version: 1.2.0"
```

```json
{
  "flags": {
    "dark_mode": false,
    "feedback_enabled": true,
    "force_update_enabled": true,
    "maintenance_mode": false,
    "minimum_app_version": "1.1.2"
  }
}
```

**Flutter:**

```dart
Future<Map<String, dynamic>> getFlags({String? userId}) async {
  final headers = <String, String>{
    'Device-ID': await getDeviceId(),
    'Platform': Platform.isAndroid ? 'android' : 'ios',
    'App-Version': packageInfo.version,
    if (userId != null) 'User-ID': userId,
  };

  final response = await http.get(
    Uri.parse('$baseUrl/api/v1/flags?app=bettercity-flutter'),
    headers: headers,
  );
  if (response.statusCode != 200) {
    throw Exception('Failed to load flags: ${response.statusCode}');
  }
  return json.decode(response.body)['flags'] as Map<String, dynamic>;
}
```

## Targeting

As queries do GOFF usam a targeting key (`targetingKey`) e os atributos acima (`user_id`, `device_id`, `platform`, `app_version`). Backends montam o próprio contexto de avaliação no SDK.

Por usuário:
```yaml
feedback_enabled:
  variations:
    enabled: true
    disabled: false
  targeting:
    - query: targetingKey eq "42"
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

## Configuração da API

Carregada de `config/{APP_ENV}.yaml` (default: `local`). Variáveis de ambiente sobrescrevem o YAML (ex.: `GOFF_ENDPOINT` → `goff.endpoint`). Um `.env` no diretório atual é lido para desenvolvimento local, mas variáveis já definidas têm prioridade.

| Campo | Descrição | Default |
|-------|-----------|---------|
| `app.port` | Porta HTTP | obrigatório |
| `app.log_level` | `debug`, `info`, `warn`, `error` | `info` |
| `app.cors_origins` | Origens permitidas (só apps web precisam) | `["*"]` se vazio |
| `app.rate_limit` | Requests por segundo por IP em `/api/v1` | `100` |
| `goff.endpoint` | URL do relay | obrigatório |

## Docker

Duas imagens, construídas no CI depois dos testes:

- **`Dockerfile`**: relay. Um stage Go roda `relay-config` para gerar a config (um flag set por app) e a imagem GOFF fixada recebe `flags/` e a config gerada.
- **`Dockerfile.api`**: build multi-stage da Flag API, com `config/` e `flags/` embutidos.

As flags vão embutidas nas duas imagens, então mudar uma flag exige novo build e deploy.

Deploy: [`DEPLOYMENT.md`](DEPLOYMENT.md).
