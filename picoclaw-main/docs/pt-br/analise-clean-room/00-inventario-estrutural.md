# Inventario Estrutural do Repositorio

## Escopo

Este inventario cobre o repositorio `picoclaw-main` do ponto de vista de engenharia, navegacao e relevancia para clean room design. O objetivo nao e listar 1064 arquivos com o mesmo peso. O objetivo e deixar claro onde esta o codigo que define comportamento, onde estao os ativos operacionais e quais areas sao majoritariamente suporte, documentacao ou distribuicao.

## Contagem por diretorio de topo

Contagem observada de arquivos:

| Area | Arquivos |
| --- | ---: |
| raiz | 22 |
| `.github` | 12 |
| `assets` | 23 |
| `cmd` | 94 |
| `config` | 1 |
| `docker` | 8 |
| `docs` | 170 |
| `examples` | 2 |
| `pkg` | 479 |
| `scripts` | 5 |
| `web` | 238 |
| `workspace` | 15 |

Leitura pratica:

- o centro funcional do sistema esta em `pkg/`
- `web/` e uma segunda grande superficie, nao um detalhe auxiliar
- `docs/` e volumoso por causa de traducao e guias por canal
- `cmd/` e pequeno em volume relativo, mas importante por definir entradas operacionais

## Distribuicao interna de `pkg/`

| Subpacote | Arquivos |
| --- | ---: |
| `channels` | 109 |
| `tools` | 58 |
| `providers` | 55 |
| `agent` | 41 |
| `seahorse` | 25 |
| `commands` | 24 |
| `audio` | 22 |
| `config` | 19 |
| `utils` | 18 |
| `routing` | 10 |
| `migrate` | 10 |
| `skills` | 10 |
| `auth` | 10 |
| `isolation` | 8 |
| `logger` | 6 |
| `credential` | 6 |
| `memory` | 5 |
| `devices` | 5 |
| `session` | 5 |
| `pid` | 4 |
| `bus` | 3 |
| `gateway` | 3 |
| `mcp` | 3 |
| `media` | 3 |
| `state` | 2 |
| `updater` | 2 |
| `fileutil` | 2 |
| `cron` | 2 |
| `health` | 2 |
| `identity` | 2 |
| `heartbeat` | 2 |
| `constants` | 1 |
| `tokenizer` | 1 |
| `pkg-root` | 1 |

Leitura pratica:

- o projeto e fortemente concentrado em canais, tools e providers
- o runtime do agente e pequeno em numero de arquivos comparado a integracoes, mas concentra complexidade maior
- `seahorse` e um subsistema proprio, nao mero utilitario

## Distribuicao interna de `web/`

| Area | Arquivos |
| --- | ---: |
| `web/frontend` | 165 |
| `web/backend` | 69 |
| `web-root` | 4 |

Leitura pratica:

- o launcher web e majoritariamente frontend
- o backend do launcher ainda e robusto e comporta auth, API, proxy e gestao de subprocesso

## Arquivos de raiz com impacto tecnico direto

| Arquivo | Papel |
| --- | --- |
| `go.mod` | define stack, dependencias e plataforma base Go 1.25.9 |
| `go.sum` | lock de dependencias Go |
| `Makefile` | principal orquestrador local de build, cross-build e launcher |
| `.goreleaser.yaml` | pipeline de release, empacotamento, imagens Docker e multiplataforma |
| `.golangci.yaml` | politica de lint, formatacao e relaxamentos atuais |
| `.env.example` | template de variaveis de ambiente |
| `.dockerignore` | higiene de build de imagem |
| `.gitignore` | exclusoes do repositorio |
| `README.md` e traducoes | superficie publica e onboarding |
| `ROADMAP.md` | prioridades futuras e intencao de produto |
| `CONTRIBUTING.md` | regras de contribuicao |
| `LICENSE` | licenca MIT |

## Inventario de `cmd/`

### `cmd/picoclaw/`

- `main.go`: CLI principal, registra subcomandos e inicia runtime
- `main_test.go`: cobertura do entrypoint CLI
- `dns_noresolv.go`: comportamento de DNS em cenarios especificos
- `internal/agent/*`: comando e helpers do modo agent
- `internal/auth/*`: login, logout, status, modelos e auth flows da CLI
- `internal/cliui/*`: renderizacao de help, status e erros no terminal
- `internal/cron/*`: CRUD e toggles das tarefas agendadas
- `internal/gateway/*`: comando do gateway
- `internal/migrate/*`: migracao de configuracao
- `internal/model/*`: comandos ligados a modelos
- `internal/skills/*`: install, list, search, remove e show de skills
- `internal/status/*`: status operacional
- `internal/version/*`: versionamento e saida de versao

### `cmd/picoclaw-launcher-tui/`

- `main.go`: entrypoint do launcher TUI
- `README.md`: documentacao da TUI
- `config/config.go`: carga e persistencia do `tui.toml`
- `ui/app.go`: aplicacao raiz da TUI
- `ui/home.go`: tela inicial
- `ui/schemes.go`: providers e esquemas de configuracao
- `ui/users.go`: gestao de usuarios/credenciais
- `ui/models.go`: dados e selecao de modelos
- `ui/channels.go`: configuracao de canais
- `ui/gateway.go`: controle do gateway

### `cmd/membench/`

- `main.go`: entrypoint do benchmark
- `ingest.go`, `eval.go`, `metrics.go`, `locomo.go`, `legacy_store.go`: utilitarios de medicao de memoria, ingestao e avaliacao
- `*_test.go`: cobertura do benchmark e das metricas

## Inventario de areas operacionais pequenas

### `config/`

- `config.example.json`: contrato de configuracao de referencia para operadores e clean room readers

### `docker/`

- `Dockerfile`: imagem base
- `Dockerfile.full`: imagem mais pesada
- `Dockerfile.goreleaser`: build de release
- `Dockerfile.goreleaser.launcher`: build do launcher
- `Dockerfile.heavy`: variante adicional
- `docker-compose.yml`: perfis `agent`, `gateway`, `launcher`
- `docker-compose.full.yml`: compose estendido
- `entrypoint.sh`: first-run setup e bootstrap

### `scripts/`

- `build-macos-app.sh`: empacotamento macOS
- `setup.iss`: instalador Windows Inno Setup
- `icon.icns`: recurso de icone
- `test-docker-mcp.sh`: validacao de MCP em Docker
- `test-irc.sh`: teste de canal IRC

### `examples/`

- `pico-echo-server/main.go`: servidor WebSocket minimo para o protocolo Pico
- `pico-echo-server/README.md`: explicacao do exemplo

### `workspace/`

- `AGENT.md`: identidade/comportamento do agente
- `USER.md`: contexto do usuario
- `SOUL.md`: estilo e tom
- `memory/MEMORY.md`: memoria local
- `skills/*`: skills versionadas de exemplo, incluindo `weather`, `github`, `hardware`, `tmux`, `summarize`, `skill-creator`, `agent-browser`

## Estrutura de `web/`

### `web/backend/`

Familias principais:

- entrypoint e lifecycle: `main.go`, `app_runtime.go`
- embedded assets: `embed.go`, `dist/.gitkeep`
- i18n e systray: `i18n.go`, `systray*.go`
- configuracao do launcher: `launcherconfig/*`
- auth do dashboard: `dashboardauth/*`
- middleware HTTP: `middleware/*`
- API REST: `api/*`
- utilitarios: `utils/*`
- recursos visuais e winres: `icon.*`, `winres/*`

### `web/frontend/`

Familias principais:

- infraestrutura: `package.json`, `vite.config.ts`, `tsconfig*.json`, `eslint.config.js`, `prettier.config.js`
- entrada: `src/main.tsx`, `src/index.css`
- rotas: `src/routes/*`
- componentes: `src/components/*`
- hooks: `src/hooks/*`
- cliente de API: `src/api/*`
- features de chat: `src/features/chat/*`
- estado local: `src/store/*`
- i18n: `src/i18n/*`
- assets publicos: `public/*`

## Estrutura de `docs/`

`docs/` contem dois tipos de documento muito diferentes:

1. documentos tecnicos de referencia
2. traducoes e guias por canal

Blocos mais relevantes para engenharia:

- `configuration.md`
- `providers.md`
- `docker.md`
- `debug.md`
- `security_configuration.md`
- `sensitive_data_filtering.md`
- `credential_encryption.md`
- `config-versioning.md`
- `cron.md`
- `subturn.md`
- `steering.md`
- `spawn-tasks.md`
- `rate-limiting.md`
- `hooks/*`
- `design/*`
- `channels/*/README*.md`

Blocos repetitivos ou editoriais:

- traducoes em `pt-br/`, `zh/`, `ja/`, `vi/`, `fr/`, `my/`
- guias equivalentes por canal em varios idiomas

## O que importa mais para clean room

Pastas de maior valor de engenharia reversa:

- `cmd/picoclaw`
- `pkg/agent`
- `pkg/gateway`
- `pkg/config`
- `pkg/providers`
- `pkg/channels`
- `pkg/tools`
- `pkg/session`
- `pkg/memory`
- `pkg/seahorse`
- `web/backend`
- `web/frontend`

Pastas de valor secundario, mas ainda util:

- `docker`
- `.github/workflows`
- `workspace`
- `examples`
- `docs/design`

## Leitura recomendada a partir deste inventario

1. `01-runtime-core.md`
2. `02-integracoes-externas.md`
3. `03-launchers-e-superficies.md`
4. `04-operacao-build-release.md`
5. `05-guia-de-qa-e-clean-room.md`
