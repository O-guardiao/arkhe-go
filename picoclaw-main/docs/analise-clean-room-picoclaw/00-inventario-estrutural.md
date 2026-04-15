# 00 Inventario Estrutural

## Escala do Repositorio

- Total aproximado de arquivos: 1076
- Go: 627
- Testes Go (`*_test.go`): 242
- Markdown: 218
- Web (`.ts`, `.tsx`): 140
- `pkg/`: 479 arquivos
- `web/`: 238 arquivos
- `docs/`: 177 arquivos
- `cmd/`: 94 arquivos

## Pastas de Topo

| Area | Volume | Papel | Prioridade clean room |
| --- | ---: | --- | --- |
| `cmd/` | 94 | Entrypoints executaveis | Alta |
| `pkg/` | 479 | Runtime, canais, modelos, ferramentas, persistencia | Critica |
| `web/` | 238 | Launcher, dashboard, WS proxy, frontend SPA | Alta |
| `docs/` | 177 | Comportamento documentado e operacao | Media |
| `config/` | 1 | Exemplo de configuracao publica | Alta |
| `docker/` | 8 | Deploy e bootstrap em container | Media |
| `scripts/` | 5 | Automacao auxiliar | Media |
| `workspace/` | 15 | Agente/skills embarcados | Media |
| `assets/` | 23 | Imagens, logos, midia de demonstracao | Baixa |
| `examples/` | 2 | Exemplo funcional minimo | Media |

## Superficies Canonicas

### 1. Runtime e boot

- `cmd/picoclaw/`
- `pkg/gateway/`
- `pkg/agent/`
- `pkg/session/`
- `pkg/state/`
- `pkg/bus/`

### 2. Integracoes e extensibilidade

- `pkg/channels/`
- `pkg/providers/`
- `pkg/tools/`
- `pkg/skills/`
- `pkg/mcp/`
- `pkg/credential/`

### 3. Operacao local e UX

- `web/backend/`
- `web/frontend/`
- `cmd/picoclaw-launcher-tui/`

### 4. Contratos auxiliares

- `config/config.example.json`
- `docs/`
- `workspace/`
- `docker/`
- `Makefile`
- `.goreleaser.yaml`

## Breakdown de `cmd/`

| Pasta | Arquivos | Papel |
| --- | ---: | --- |
| `cmd/picoclaw/` | 74 | CLI principal, onboarding, gateway, auth, status |
| `cmd/picoclaw-launcher-tui/` | 10 | Launcher TUI simples para subir agent/gateway |
| `cmd/membench/` | 10 | Benchmark/memoria, nao faz parte do produto final |

Leitura clean room:

- `cmd/picoclaw/` e codigo comportamental;
- `cmd/picoclaw-launcher-tui/` e superficie auxiliar, nao motor central;
- `cmd/membench/` e ferramental de medicao, nao contrato de produto.

## Breakdown de `pkg/`

| Pasta | Arquivos | Papel principal |
| --- | ---: | --- |
| `agent/` | 41 | Loop de conversa, eventos, contexto, hooks, subturn |
| `channels/` | 109 | Adapters de transporte e manager de entrega |
| `providers/` | 55 | Integracao com LLMs, fallback, rate limit |
| `tools/` | 58 | Ferramentas chamaveis pelo agente |
| `commands/` | 24 | Comandos slash e runtime command-like |
| `config/` | 19 | Modelo de config, defaults, serializacao segura |
| `seahorse/` | 25 | Base de dados/estado auxiliar usado por certas integracoes |
| `audio/` | 22 | ASR/TTS |
| `routing/` | 10 | Modelo leve vs pesado |
| `migrate/` | 10 | Migracao de configuracao e compatibilidade |
| `auth/` | 10 | OAuth e autent. por provider |
| `credential/` | 6 | Resolucao e criptografia de segredos |
| `memory/` | 5 | Store local JSONL/estado auxiliar |
| `session/` | 5 | Persistencia de sessao |
| `state/` | 2 | Ultimo canal/chat ativo |
| `gateway/` | 3 | Orquestracao de runtime e startup |
| `health/` | 2 | Endpoints de health/reload |
| `bus/` | 3 | Envelope de mensagens entre loop e canais |
| `devices/` | 5 | Eventos de hardware |
| `skills/` | 10 | Descoberta e ativacao de skills |
| `mcp/` | 3 | Spawn e gestao de servidores MCP |

## Breakdown de `web/`

| Pasta | Arquivos | Papel |
| --- | ---: | --- |
| `web/backend/` | 69 | API local, auth, gateway lifecycle, session browser, proxy WS |
| `web/frontend/` | 165 | SPA React/TanStack para dashboard e chat |
| `web/README.md` | 1 | Explica launcher web |
| `web/Makefile` | 1 | Build do frontend/backend |

## Papel das Pastas Auxiliares

### `docs/`

Nao e codigo executavel, mas ajuda a definir o comportamento esperado. Os clusters mais relevantes sao:

- `configuration.md`, `config-versioning.md`, `providers.md`, `tools_configuration.md`
- `credential_encryption.md`, `security_configuration.md`, `sensitive_data_filtering.md`
- `channels/`, `hooks/`, `migration/`, `design/`, `agent-refactor/`
- traducoes `pt-br/`, `fr/`, `ja/`, `vi/`, `zh/`, `my/`

### `workspace/`

Contem artefatos de produto e nao apenas fixture de desenvolvimento:

- `AGENT.md`
- `USER.md`
- `SOUL.md`
- `skills/`
- `memory/`

Uma reimplementacao clean room pode substituir essa pasta por outro formato, mas precisa preservar a ideia de um workspace default com prompts, skills e memoria local.

### `assets/`

Baixo valor para clean room. Sao logos, capturas, GIFs e imagens de marketing.

### `examples/`

Baixo volume, mas util para comportamento observavel de integracao minima.

## Leitura por Estrato de Comportamento

### Estrato 1: produto canonico

- `cmd/picoclaw`
- `pkg/agent`
- `pkg/gateway`
- `pkg/channels`
- `pkg/providers`
- `pkg/tools`
- `web/backend`

### Estrato 2: comportamento operacional

- `pkg/config`
- `pkg/credential`
- `pkg/migrate`
- `pkg/health`
- `pkg/session`
- `pkg/state`
- `web/frontend`

### Estrato 3: suporte, docs e exemplos

- `docs`
- `assets`
- `examples`
- `scripts`
- `docker`

## Orientacao de Clean Room

Nao reproduza a arvore literal do repositorio. A equivalencia deve ser por papel:

- transporte/adapters;
- motor de conversa;
- politicas de modelo;
- execucao de ferramentas;
- persistencia local;
- painel/launcher.

Se sua nova base tiver esses contratos organizados de outro jeito, a paridade de comportamento continua valida.
