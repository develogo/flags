# Glossário

Serviço de feature flags compartilhado entre projetos pessoais (BetterCity e outros).

## Termos

**App**
Consumidor de flags com nome único global e plano (ex.: `bettercity-flutter`, `bettercity-api`).
O nome carrega o projeto; não existe nível "projeto" acima do app. Cada app tem exatamente um
arquivo de definição de flags.
Um app só enxerga as próprias flags: nomes de flag podem se repetir entre apps sem conflito.
Esse isolamento é organizacional, não é segurança: quem sabe o nome do app consegue ler as flags
dele. Por isso, flag nunca guarda segredo.
_Evitar_: projeto, tenant, cliente.

**App público**
App cujas flags podem ser lidas pelo endpoint público (sem autenticação, fora da rede interna).
Um app só é público se for liberado explicitamente; por padrão, as flags ficam acessíveis apenas
na rede interna. Kill switches de backend nunca pertencem a um app público.
_Evitar_: served app.

**Flag**
Valor configurável pertencente a um único app, com nome em snake_case, variations de um mesmo
tipo escalar e um valor de fallback.

**Arquivo de flags**
O YAML no formato GOFF que define todas as flags de um app. É a única fonte de definição de flags.
_Evitar_: "arquivo Markdown", registry.

**Contexto de targeting**
Os atributos que o chamador envia para a avaliação (identificador de usuário ou dispositivo,
plataforma, versão do app). Declarados pelo cliente, não verificados: o serviço não autentica
usuários.

**Rota legada**
O comportamento de compatibilidade para os builds já instalados do Flutter BetterCity: um pedido
sem app, ou com o nome antigo `flutter`, é atendido como `bettercity-flutter`.
