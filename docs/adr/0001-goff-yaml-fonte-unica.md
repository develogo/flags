# ADR 0001 — Os YAML do GOFF são a única fonte de definição de flags

**Data:** 2026-09-11 · **Status:** aceito; itens 3 e 4 alterados pela
[ADR 0002](0002-servico-multi-app-flag-set-por-app.md)

## Contexto

Até aqui a API mantinha um registry próprio em `config/flags.yaml` (nome, tipo,
default por app) que precisava ficar "em sincronia" com `flags/apps/<app>.yaml`
(o arquivo que o relay GOFF carrega). O mesmo fato vivia em dois arquivos com
dois formatos. Na prática a sincronia era manual e já havia drift:
`force_update_enabled` estava `false` no registry e `enabled` no GOFF, ou seja,
com o relay fora o Flutter receberia o oposto do que o GOFF entregaria.

## Decisão

1. A API lê `flags/apps/<app>.yaml` diretamente. O tipo do flag é inferido dos
   valores de `variations`; o fallback é o valor de `defaultRule.variation`.
2. Restrições impostas no startup (a API recusa subir se violadas):
   - `variations` não vazio, todos os valores do mesmo tipo escalar
     (`bool`, `string`, `int`, `float`); `int` não é promovido a `float`.
   - `defaultRule.variation` obrigatório e apontando para uma variation
     declarada. `defaultRule.percentage` sem `variation` não é aceito, porque a
     API precisa de um único valor de fallback.
3. A API serve apenas os apps listados em `ServedApps`
   (`internal/services/registry.go`), hoje só `flutter`. `flags/apps/api.yaml`
   e `flags/shared.yaml` são consumidos por backends via SDK do relay e não são
   expostos por este endpoint — evitar publicar kill switches de backend num
   endpoint sem autenticação.
4. O diretório `flags/apps` é constante (relativo ao cwd); não existe chave de
   config para ele. `Dockerfile.api` copia `flags/` para a imagem.

## Consequências

- Adicionar um flag = editar um único arquivo. Drift entre registry e GOFF é
  impossível por construção.
- A interface `FlagRegistry.GetFlagsForApp` não mudou; handlers, mocks e
  testes não foram afetados.
- Servir um novo app mobile exige adicionar o nome em `ServedApps` (uma linha)
  e criar `flags/apps/<app>.yaml`.
- Flags com rollout por porcentagem no `defaultRule` precisam mover a
  porcentagem para uma regra em `targeting` e manter um `defaultRule.variation`.
- Pendência conhecida, fora desta decisão: `maintenance_mode` existe em
  `flags/apps/flutter.yaml` e `flags/shared.yaml` com variations diferentes; o
  relay mescla os retrievers e o último vence.
