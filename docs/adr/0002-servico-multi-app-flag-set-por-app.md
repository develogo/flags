# ADR 0002 — Serviço multi-app: um flag set por app, sem autenticação

**Data:** 2026-09-14 · **Status:** aceito · **Altera:** ADR 0001 (itens 3 e 4)

## Contexto

Este serviço nasceu para o BetterCity: um relay GOFF que carrega vários arquivos num único
namespace, e uma API que serve o Flutter com Keycloak (realm `bettercity`) opcional. A ideia é
reaproveitá-lo em outros projetos pessoais para não subir um GOFF novo a cada projeto. Tudo roda
na rede interna; só a API fica exposta publicamente, para os apps mobile.

Com vários projetos, o namespace único não funciona mais. O relay mescla os retrievers e, quando
dois arquivos têm a mesma flag, o último vence em silêncio. Isso já acontece com
`maintenance_mode` em `flutter.yaml` e `shared.yaml`.

## Decisão

1. **App** é a unidade: nome plano e único global (`bettercity-flutter`, `bettercity-api`), um
   arquivo `flags/apps/<app>.yaml` por app, sem nível "projeto".
2. Cada app vira um **flag set** do relay GOFF. A API key do flag set é o **nome do app**. Ela
   funciona como seletor de namespace, não como segredo. O relay gera os flag sets no start a
   partir dos arquivos em `flags/apps/`. Para criar um app, basta criar o arquivo.
3. Não há autenticação. Quem sabe o nome do app lê as flags dele, então flag nunca guarda
   segredo. O isolamento entre apps é organizacional.
4. O JWT/Keycloak sai da API. O targeting usa os headers `Device-ID`, `Platform`, `App-Version` e
   `User-ID` (opcional), declarados pelo cliente e não verificados.
5. O endpoint público continua servindo só os apps listados em `ServedApps` (**App público**).
   No boot, a API valida apenas esses apps. Um teste no CI valida todos os arquivos.
6. `/api/v1/flags` com `app` vazio ou `flutter` resolve para `bettercity-flutter`. Isso preserva
   os builds já instalados.
7. `flags/shared.yaml` e `flags/apps/api.yaml` se fundem em `bettercity-api.yaml`.

## Alternativas descartadas

- **API key secreta por app:** operacionalmente pesada (gerar, distribuir, rotacionar) para
  flags que não são segredo, numa rede interna.
- **Namespace único com nomes de flag globalmente únicos:** impede cada app de ter a própria
  `maintenance_mode`.
- **Prefixo no nome da flag:** polui o nome que o cliente consome.
- **Backends lendo tudo pela nossa API:** perde o SDK OpenFeature e a avaliação no backend.
- **Emissores OIDC por projeto:** exige mais configuração e código para um targeting por usuário
  que hoje nenhuma flag usa.

## Consequências

- Backends precisam configurar `APIKey: "<app>"` no provider GOFF. Sem isso, o relay rejeita a
  requisição e o backend cai nos defaults do código. A migração do BetterCity foi feita junto com
  o relay, aceitando alguns minutos de defaults.
- Qualquer cliente pode declarar qualquer `User-ID`. Targeting por usuário não serve para
  controle de acesso.
- Voltar a exigir autenticação obriga todos os clientes a mudar de configuração.
