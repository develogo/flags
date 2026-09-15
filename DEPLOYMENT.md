# Guia de Deploy

O serviço tem dois containers: o **relay GOFF** e a **Flag API**. Um único deploy atende todos os apps (ver [ADR 0002](docs/adr/0002-servico-multi-app-flag-set-por-app.md)).

## Topologia

```
internet ──> hostname atual do BetterCity ─┐
internet ──> novo hostname genérico ───────┴─> proxy reverso ──> Flag API ──> relay
                                                                               ▲
                                            backends na rede interna ──SDK─────┘
```

- **Flag API**: única parte exposta publicamente. Os dois hostnames (o antigo do BetterCity e o novo genérico) apontam para o **mesmo serviço**. O hostname antigo continua existindo porque os builds instalados do Flutter o usam. DNS e proxy reverso são configurados na operação, fora deste repo.
- **Relay**: só na rede interna, sem DNS público nem porta exposta. Backends que precisam de flags rodam na mesma rede e usam `http://relay:1031`.
- **Sem autenticação e sem segredos.** Não há Keycloak, JWT nem secret de API key: a API key de cada flag set é o nome do app.

## Pipeline

No push para `main` (ou tag `v*`), o `ci.yml`:

1. Roda `go test ./... -race` e `go vet`. Os testes validam o arquivo de flags de todos os apps, então um arquivo quebrado bloqueia as imagens.
2. Builda e publica as duas imagens:
   - `Dockerfile` (relay): um stage Go roda `go run . relay-config`, que gera a config do relay com **um flag set por arquivo em `flags/apps/`**. A imagem final (`gofeatureflag/go-feature-flag`, versão fixada) recebe `flags/` e a config gerada. Não existe config do relay versionada.
   - `Dockerfile.api` (Flag API): binário Go com `config/` e `flags/` embutidos.
3. Abre um PR em `develogo/stacks` atualizando as imagens na stack. O deploy acontece quando esse PR é aplicado.

As flags ficam embutidas nas imagens. Mudar uma flag ou adicionar um app é sempre commit, CI e deploy.

## Variáveis de ambiente da API

| Variável | Produção |
|----------|----------|
| `APP_ENV` | `production` (carrega `config/production.yaml`) |
| `GOFF_ENDPOINT` | Opcional; o YAML já usa `http://relay:1031` |
| `APP_PORT`, `APP_LOG_LEVEL`, `APP_RATE_LIMIT` | Opcionais; sobrescrevem o YAML |

O relay não recebe variáveis: tudo vem da config gerada no build.

## Local (Docker Compose)

```bash
docker network create flags_local   # uma vez
make up                             # builda relay + API na primeira vez; use --build depois de mudar flags
```

```bash
curl http://localhost:1324/health
curl http://localhost:1324/ready

curl "http://localhost:1324/api/v1/flags?app=bettercity-flutter" \
  -H "Device-ID: 550e8400-e29b-41d4-a716-446655440000" \
  -H "Platform: android" \
  -H "App-Version: 1.2.0"

# Config gerada do relay, sem Docker
go run . relay-config
```

## Verificação depois do deploy

Nos **dois** hostnames:

```bash
curl https://<hostname>/ready
curl https://<hostname>/api/v1/flags              # rota legada -> bettercity-flutter
curl "https://<hostname>/api/v1/flags?app=flutter" # rota legada -> bettercity-flutter
```

As duas chamadas de flags devem retornar as flags de `flags/apps/bettercity-flutter.yaml`. Nos backends, confira que os logs não mostram falha de avaliação (key ausente ou errada faz o SDK cair nos defaults do código).

## Rollback

Reverta o PR de imagens na stack em `develogo/stacks` (ou aponte as imagens para a tag anterior) e reaplique. Relay e API podem voltar juntos: as imagens de um mesmo commit sempre concordam sobre quais apps existem.

As imagens até a renomeação genérica foram publicadas como `develogo/bettercity-flags` (relay) e `develogo/better-feature-flag` (API), na stack `stacks/bettercity-feature-flag`; um rollback para antes dela aponta para esses nomes.

## Troubleshooting

**`/ready` retorna `unavailable`**: a API não consegue avaliar no relay. Confira `GOFF_ENDPOINT` e se os dois containers estão na mesma rede.

**Backend sempre recebe os defaults do código**: a avaliação vai sem targeting key (o provider GOFF exige uma), a `APIKey` do provider não é o nome de um app existente, ou o arquivo `flags/apps/<app>.yaml` não estava na imagem do relay em produção. Veja os flag sets gerados com `go run . relay-config`.

**`400 Unknown application`**: o app não está em `ServedApps` (`internal/services/registry.go`) ou o nome está errado. Apps privados e inexistentes recebem a mesma resposta de propósito.

**API não sobe**: o arquivo de um app público está ausente ou inválido. O erro de boot indica o app (e a flag, quando o problema é numa definição).

## Checklist de produção

- [ ] `app.cors_origins` só com as origens web necessárias
- [ ] Relay sem porta ou DNS públicos
- [ ] HTTPS nos dois hostnames da API
- [ ] Health check do orquestrador/proxy apontando para `/ready`
- [ ] `app.rate_limit` adequado ao tráfego
- [ ] Nenhum kill switch de backend num app listado em `ServedApps`
