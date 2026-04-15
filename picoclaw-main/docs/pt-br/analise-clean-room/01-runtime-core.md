# Runtime Core

## Escopo

Este documento cobre a parte do repositorio que realmente decide comportamento do produto:

- `cmd/picoclaw`
- `pkg/gateway`
- `pkg/agent`
- `pkg/config`
- `pkg/bus`
- `pkg/session`
- `pkg/memory`
- `pkg/seahorse`
- `pkg/routing`
- `pkg/commands`
- `pkg/state`

## 1. Entradas do runtime

### `cmd/picoclaw/main.go`

Esse arquivo define a CLI raiz via Cobra e registra os subcomandos:

- onboard
- agent
- auth
- gateway
- status
- cron
- migrate
- skills
- model
- update
- version

Contrato observavel relevante:

- a CLI e uma superficie publica do produto
- o binario `picoclaw` concentra setup, operacao e administracao
- flags de cor e help customizado fazem parte da UX, nao da logica central

### `cmd/picoclaw/internal/gateway/command.go`

O subcomando `gateway` delega para `pkg/gateway.Run(...)` e aceita flags operacionais como:

- `--debug`
- `--no-truncate`
- `--allow-empty` (`-E`)

O `-E` e importante porque o launcher web depende dele para subir o gateway mesmo quando ainda nao ha modelo padrao configurado.

## 2. Gateway como orquestrador

### Arquivos principais

- `pkg/gateway/gateway.go`: runtime principal, startup, reload e shutdown
- `pkg/gateway/gateway_test.go`: cobertura do orquestrador
- `pkg/gateway/channel_matrix.go`: catalogo auxiliar de capacidades de canais

### Responsabilidades do gateway

- inicializar panic log e file logging
- carregar `config.json`
- validar precondicoes de configuracao
- criar PID file e garantir singleton
- resolver provider inicial
- instanciar `MessageBus`
- criar `AgentLoop`
- subir canais, cron, heartbeat, health server, media store e devices
- lidar com hot reload e reload manual
- encerrar tudo com timeout gracioso

### Contrato do gateway para clean room

Nao copie a forma exata como o gateway empilha servicos. O que precisa ser preservado e:

- existencia de um runtime daemonizado
- validacao previa de configuracao
- resolucao de provider/modelo antes de processar trafego real
- capacidade de subir/descer canais e servicos auxiliares
- shutdown previsivel e observavel

## 3. AgentLoop como coracao do sistema

### Arquivos centrais em `pkg/agent/`

#### Arquivos estruturais

- `instance.go`: instancia configurada do agente
- `definition.go`: carrega definicoes do agente e arquivos como `AGENT.md`, `SOUL.md`, `USER.md`
- `registry.go`: registry de agentes
- `model_resolution.go`: resolucao de modelo, candidates e fallbacks

#### Loop e turnos

- `loop.go`: loop principal de processamento
- `turn.go`: maquina de estado de turno
- `subturn.go`: execucao filha/concorrente
- `steering.go`: injecao de steering em tempo de execucao
- `thinking.go`: configuracao de thinking levels

#### Contexto

- `context.go`: montagem do contexto
- `context_manager.go`: estrategia plugavel de contexto
- `context_legacy.go`: estrategia legada
- `context_budget.go`: orcamento de contexto
- `context_cache_test.go`, `context_budget_test.go`, `context_manager_test.go`: cobertura do subsistema
- `context_seahorse.go`: adaptacao ao backend Seahorse
- `context_seahorse_unsupported.go`: fallback para ambientes sem suporte

#### Hooks e eventos

- `eventbus.go`: bus interno de eventos
- `events.go`: tipos de eventos
- `hooks.go`: contratos de hook
- `hook_mount.go`: lifecycle de hooks
- `hook_process.go`: ponte IPC/JSON-RPC para hooks externos

#### MCP e media

- `loop_mcp.go`: inicializacao de runtime MCP
- `loop_media.go`: tratamento de media e anexos

### Fluxo funcional do AgentLoop

Em alto nivel, o turno executa:

1. resolve sessao, canal, sender e metadados
2. verifica comandos slash
3. injeta steering pendente se existir
4. monta contexto respeitando budget
5. escolhe modelo efetivo, incluindo routing light/heavy
6. chama o provider
7. interpreta tool calls
8. executa tools, possivelmente em subturns
9. persiste mensagens e dispara compactacao/sumario quando necessario
10. publica resposta no `MessageBus`

### Riscos de QA aqui

- race conditions entre steering, tools e subtuns
- turn state mal encerrado sob timeout, cancelamento ou erro de hook
- vazamento de contexto entre sessoes
- inconsistencias entre fallback de provider e routing de modelo
- regressao silenciosa em hooks, porque eles alteram o pipeline de execucao

## 4. Configuracao como contrato publico

### Arquivos centrais em `pkg/config/`

- `config.go`: struct `Config` e filtros de dados sensiveis
- `config_struct.go`: definicoes auxiliares
- `defaults.go`: defaults
- `gateway.go`: subconfig do gateway
- `security.go`: filtro e cache de segredos
- `migration.go`: migracao v1 -> v2
- `envkeys.go`: variaveis de ambiente
- `version.go`: versao de build
- `config_old.go`: estrutura antiga para migracao/compatibilidade
- `example_security_usage.go`: uso de seguranca

### Partes do schema com maior peso funcional

- `agents`
- `bindings`
- `session`
- `channels`
- `model_list`
- `gateway`
- `hooks`
- `tools`
- `heartbeat`
- `devices`
- `voice`

### Contrato de configuracao para clean room

Se voce for reimplementar, trate estes itens como contrato de produto:

- existencia de um arquivo `config.json`
- nomes e significados das areas principais do schema
- capacidade de override via env vars
- migracao de schema como preocupacao de compatibilidade

O que nao precisa ser igual:

- organizacao interna das structs
- helpers de default e cache
- detalhes do loader JSON/YAML

## 5. Bus, comando e estado

### `pkg/bus/`

- `bus.go`: canais buffered para inbound/outbound/media/audio
- `types.go`: structs de mensagem
- `bus_test.go`: cobertura da infraestrutura

Esse pacote e a costura entre canais e loop do agente. Contrato relevante: mensagens estruturadas, separacao inbound/outbound e transporte thread-safe em processo.

### `pkg/commands/`

Arquivos principais:

- `executor.go`: execucao de comandos
- `registry.go`: registro de handlers
- `definition.go`: definicao dos comandos
- `request.go`: request normalizada
- `builtin.go`: comandos builtin
- `cmd_*.go`: implementacoes de comandos como `help`, `list`, `reload`, `use`, `show`, `start`, `subagents`, `switch`, `clear`, `check`
- `runtime.go`: acesso ao runtime pelo executor

Contrato relevante:

- slash commands sao parte do comportamento do produto
- comando desconhecido pode virar passthrough para o LLM
- comandos conhecidos podem ter tratamento por canal e por runtime

### `pkg/state/`

- `state.go`: estado persistente simples
- `state_test.go`: cobertura

Esse pacote e pequeno, mas operacionalmente importante para lembrar ultimo canal, chat ou estado auxiliar entre execucoes.

## 6. Persistencia: tres camadas, nao uma

### `pkg/session/`

- `manager.go`: sessoes em memoria, persistencia JSON por sessao
- `jsonl_backend.go`: backend JSONL complementar
- `session_store.go`: contratos

### `pkg/memory/`

- `store.go`: interface de store
- `jsonl.go`: `JSONLStore` append-only com truncacao logica
- `migration.go`: migracao de backend

### `pkg/seahorse/`

- `schema.go`: schema SQLite com FTS5 e trigram
- `store.go`: conversa, mensagens, sumarios e contexto
- `short_engine.go`: mecanismo de resumo/recuperacao curta
- `short_compaction.go`, `short_retrieval.go`, `short_assembler.go`: pipeline de compactacao e rehidracao
- `tool_grep.go`, `tool_expand.go`: busca estruturada
- `types.go`: tipos do dominio

### Leitura correta

O projeto mostra uma evolucao de persistencia. Nao existe um unico backend hegemonico simples. Para clean room, isso significa:

- o produto precisa persistir historico e contexto de modo eficiente
- a forma exata da persistencia pode ser redesenhada
- sumario, truncacao e busca semantica fazem parte do efeito esperado, nao necessariamente da mesma arquitetura interna

## 7. Routing de modelo

### `pkg/routing/`

- `router.go`: roteador principal
- `classifier.go`: classificador
- `features.go`: extracao de features
- `route.go`, `agent_id.go`, `session_key.go`: resolucoes auxiliares e binds

Contrato observavel:

- o sistema pode mandar mensagens simples para modelos mais leves
- a decisao e baseada em complexidade estrutural, nao apenas em palavras-chave
- binds por canal/conta/peer influenciam o agente efetivo

## 8. Hotspots de regressao

Se voce tivesse que priorizar revisao de regressao, comecaria por aqui:

1. `pkg/agent/loop.go`
2. `pkg/agent/turn.go`
3. `pkg/agent/subturn.go`
4. `pkg/gateway/gateway.go`
5. `pkg/config/config.go` e `migration.go`
6. `pkg/commands/executor.go`
7. `pkg/routing/router.go`
8. `pkg/seahorse/*`

Motivo: estas areas mudam comportamento sistêmico, nao apenas um adaptador local.

## 9. O que documentar mais tarde, se precisar aprofundar

- diagrama completo do ciclo de turno
- tabela de todos os eventos emitidos em `events.go`
- mapeamento detalhado entre hooks e fases do loop
- comparativo entre persistencia JSON, JSONL e Seahorse
- matriz de compatibilidade entre routing, fallback e providers
