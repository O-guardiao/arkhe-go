# 05 Guia QA Clean Room

## Findings Prioritarios

### 1. Alta - exec remoto habilitado por default combinado com canais permissivos

Evidencia:

- `pkg/config/defaults.go`: `tools.exec.allow_remote = true` e `tools.exec.enabled = true`
- `pkg/agent/instance.go`: registro automatico do exec tool
- `pkg/channels/base.go`: `allow_from` vazio aceita qualquer remetente
- `config/config.example.json`: varios canais mostram `allow_from: []`
- `pkg/tools/shell.go`: execucao real via `sh -c` ou `powershell -Command`

Impacto:

- se um operador habilitar um canal externo e mantiver defaults, um usuario remoto pode induzir execucao local com superficie maior do que o desejado;
- os regex de bloqueio reduzem risco, mas nao transformam shell arbitrario em primitivas seguras.

Orientacao clean room:

- seu design deve nascer com `remote exec` desabilitado por default;
- canais externos devem exigir allowlist explicita ou modo publico declarado;
- prefira ferramentas estruturadas a shell generica.

### 2. Media - bootstrap do launcher por token na URL

Evidencia:

- `web/backend/middleware/launcher_dashboard_auth.go`: aceita `?token=` em GET e seta cookie
- `web/backend/main.go`: monta URL local com token para abrir o navegador
- `web/backend/middleware/referrer_policy.go`: reduz, mas nao elimina, risco de exposicao inicial

Impacto:

- token pode aparecer em historico local, screenshots e logs de proxy/browser;
- o risco e menor em uso local, mas aumenta se o launcher for exposto em rede.

Orientacao clean room:

- prefira bootstrap por one-time code curto, loopback callback, token em memoria ou POST local inicial;
- nao dependa de query string como forma principal de sessao.

### 3. Baixa - `/health` e `/ready` revelam metadados sem auth

Evidencia:

- `pkg/health/server.go`: `/health` devolve PID e uptime; `/ready` devolve checks
- `pkg/gateway/gateway.go`: health server sobe no mesmo host/porta do gateway
- `pkg/config/defaults.go`: bind default e loopback, o que reduz impacto

Impacto:

- em exposicao nao local, ajuda enumeracao e observabilidade por terceiros.

Orientacao clean room:

- se for expor health fora de loopback, limite o payload publico e separe health publica de admin.

## Checklist de Paridade de Comportamento

### Runtime

- sessao persiste entre mensagens e sobrevive a restart;
- tools podem ser invocadas iterativamente;
- slash commands passam antes do LLM;
- fallback de modelo muda o resultado operacional quando um provider falha.

### Canais

- grupos, mentions, prefixes e allowlist mudam o comportamento observavel;
- placeholder/typing/streaming nao podem duplicar respostas;
- mensagem longa respeita limites por plataforma.

### Launcher

- painel sobe runtime, autentica acesso, faz proxy Pico e edita config;
- segredos nao voltam em claro para o frontend;
- restart do gateway e refletido no dashboard.

## Checklist de Identidade Propria

Sinais de copia disfarçada:

- mesmos nomes de pastas e structs sem ganho proprio;
- mesma ordem de startup;
- mesma decomposicao de hooks, context manager, fallback, channels e tools;
- mesmo naming publico para campos que poderiam ser repensados.

Sinais de implementacao autoral:

- novas fronteiras de modulo;
- contratos menores e mais explicitos;
- separacao entre transporte, motor, politicas e painel;
- runtime shell substituido por acoes estruturadas ou job primitives mais seguras.

## Checklist de Edge Cases

- config parcial com segredo omitido;
- provider primary indisponivel e fallback esgotado;
- canal habilitado sem allowlist;
- websocket do Pico em reconnect durante restart do gateway;
- sessao JSONL com linhas invalidas;
- tool longa cancelada no meio do shutdown;
- MCP server morto, travado ou sem binario.

## Criterio de Aprovacao Clean Room

Uma reimplementacao deve ser aprovada quando:

1. entrega os mesmos resultados observaveis para chat, canais, tools e painel;
2. passa pelos testes de comportamento e seguranca da matriz desta suite;
3. nao replica a mesma topologia interna do repositorio original;
4. fecha os riscos identificados acima com defaults mais defensivos do que o original.
