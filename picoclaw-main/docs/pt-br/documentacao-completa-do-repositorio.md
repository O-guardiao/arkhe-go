# Documentacao Completa do Repositorio PicoClaw

Este documento existe para reduzir o custo de entendimento do repositorio inteiro. O foco aqui nao e repetir tutorial de uso basico; e explicar como a codebase esta organizada, onde estao os pontos de entrada, quais contratos externos o sistema expoe e quais partes sao mais sensiveis para manutencao, QA e recriacao por clean room design.

Escopo deste documento:

- arquitetura de alto nivel
- mapa de diretorios e pacotes
- executaveis e fluxos de inicializacao
- launcher web e separacao de processos
- configuracao, persistencia e seguranca
- build, testes, release e operacao
- lacunas reais de documentacao e de qualidade
- guia para reimplementacao por comportamento, sem copiar a estrutura interna

Complemento importante:

- para uma leitura mais profunda, orientada a QA e clean room, veja a suite em `docs/pt-br/analise-clean-room/README.md`

## 1. Resumo Executivo

PicoClaw e um assistente de IA pessoal escrito em Go, com duas superficies principais de operacao:

- um binario principal de CLI e gateway, exposto por `picoclaw`
- um launcher web separado, que sobe um backend HTTP, autentica o dashboard, inicia ou anexa o gateway e faz proxy do canal Pico via WebSocket

O centro da aplicacao vive em `pkg/`. Os componentes mais importantes sao:

- `pkg/gateway`: orquestracao do runtime
- `pkg/agent`: loop principal do agente, contexto, steering, subturn, hooks e integracao com tools
- `pkg/channels`: adaptadores de entrada e saida para canais externos
- `pkg/providers`: abstracao de LLMs, factory, fallback e rate limiting
- `pkg/config`: schema de configuracao, defaults, migration e filtros de dados sensiveis
- `pkg/tools`: ferramentas registradas e executadas pelo agente

Ha ainda um launcher web em `web/` com backend em Go e frontend em React 19 + Vite, um launcher TUI em `cmd/picoclaw-launcher-tui/`, um benchmark de memoria em `cmd/membench/`, exemplos em `examples/` e um workspace de exemplo versionado em `workspace/`.

## 2. Mapa do Repositorio

### 2.1 Diretorios de topo

| Caminho | Papel |
| --- | --- |
| `.github/` | workflows de CI, PR, nightly, release e empacotamento |
| `assets/` | imagens, logo e material visual usado por README e launcher |
| `cmd/` | executaveis Go versionados no repo |
| `config/` | `config.example.json`, usado como referencia de schema operacional |
| `docker/` | Dockerfiles e compose para agent, gateway e launcher |
| `docs/` | documentacao funcional, operacional e por canal |
| `examples/` | exemplos de integracao, como `pico-echo-server` |
| `pkg/` | nucleo da aplicacao, dividido por responsabilidade |
| `scripts/` | suporte a empacotamento e instaladores |
| `web/` | launcher web com backend Go e frontend React |
| `workspace/` | scaffold de workspace com `AGENT.md`, `USER.md`, `SOUL.md`, `memory/` e `skills/` |

### 2.2 Binarios versionados

| Binario | Ponto de entrada | Papel |
| --- | --- | --- |
| `picoclaw` | `cmd/picoclaw/main.go` | CLI principal e gateway |
| `picoclaw-launcher` | `web/backend/main.go` | launcher web, dashboard, auth e subprocess manager |
| `picoclaw-launcher-tui` | `cmd/picoclaw-launcher-tui/main.go` | launcher TUI local |
| `membench` | `cmd/membench/main.go` | benchmark e avaliacao de memoria |

## 3. Pontos de Entrada e Superficies Publicas

### 3.1 CLI principal

O comando raiz usa Cobra em `cmd/picoclaw/main.go`. A CLI registra subcomandos para:

- onboarding inicial
- execucao do agente
- autenticacao
- gateway
- status
- cron
- migracao
- skills
- modelos
- update
- version

Na pratica, o binario `picoclaw` acumula tres papeis:

- ferramenta de setup e manutencao
- entrypoint do runtime em modo gateway
- interface de administracao do ambiente local

### 3.2 Gateway

O subcomando `picoclaw gateway` delega para `pkg/gateway.Run(...)`. Esse e o entrypoint operacional mais importante do sistema.

O gateway:

- carrega e valida a configuracao
- prepara logging, panic log e PID file
- resolve provider inicial e modelo default
- cria o `MessageBus`
- instancia o `AgentLoop`
- sobe cron, heartbeat, health server, media store, channels e demais servicos
- oferece hot reload experimental quando habilitado
- encerra de forma graciosa sob sinal do sistema

### 3.3 Launcher web

O launcher web, em `web/backend/main.go`, e outro entrypoint importante. Ele nao e apenas um frontend: e um processo de controle.

Responsabilidades do launcher:

- servir o dashboard web
- autenticar acesso ao dashboard
- fazer onboarding automatico quando a configuracao ainda nao existe
- iniciar ou anexar o subprocesso `picoclaw gateway -E`
- manter um buffer em memoria com logs do gateway
- expor APIs REST para modelos, credenciais, canais, configuracao, logs e estado do gateway
- fazer proxy do canal Pico via `/pico/ws`

### 3.4 Launcher TUI

O launcher TUI em `cmd/picoclaw-launcher-tui/main.go` e uma superficie local separada. Ele:

- localiza ou recebe o caminho de configuracao
- dispara `picoclaw onboard` se o diretorio ainda nao existe
- carrega a configuracao do launcher TUI
- sincroniza a selecao de modelo com a configuracao principal
- roda uma UI local baseada no pacote `ui`

### 3.5 Exemplo de protocolo

`examples/pico-echo-server` implementa um servidor WebSocket minimo para o canal `pico_client`. Esse exemplo e importante porque documenta o comportamento observavel do protocolo Pico sem depender do runtime inteiro.

## 4. Arquitetura em Alto Nivel

### 4.1 Visao de runtime

```text
Usuarios / Bots / Canais / Dashboard
                |
                v
      +-----------------------+
      | channel adapters      |
      | pkg/channels/*        |
      +-----------+-----------+
                  |
                  v
      +-----------------------+
      | MessageBus            |
      | pkg/bus              |
      +-----------+-----------+
                  |
                  v
      +-----------------------+
      | AgentLoop             |
      | pkg/agent             |
      +--+---------+----------+
         |         |          |
         |         |          |
         v         v          v
  +-----------+ +--------+ +-----------+
  | providers | | tools  | | sessions  |
  | pkg/...   | | pkg/...| | memory/...|
  +-----------+ +--------+ +-----------+
         |
         v
  resposta final / tool results / side effects
         |
         v
      MessageBus -> outbound -> channel adapters
```

### 4.2 Separacao launcher x gateway

```text
picoclaw-launcher (processo A)
  - backend HTTP
  - auth do dashboard
  - assets web embutidos
  - gestao do subprocesso
  - proxy /pico/ws
            |
            v
picoclaw gateway -E (processo B)
  - canais
  - agent loop
  - providers
  - cron / heartbeat / health
  - tools / sessions / memoria
```

Essa separacao e um dos contratos arquiteturais mais relevantes do projeto. O launcher web nao substitui o gateway; ele o administra.

## 5. Fluxos Principais de Execucao

### 5.1 Sequencia de startup do gateway

Com base em `pkg/gateway/gateway.go`, o startup segue esta ordem logica:

1. inicializar panic log e file logging
2. carregar `config.json`
3. rodar pre-checks de configuracao
4. aplicar nivel de log efetivo
5. criar PID file e garantir singleton
6. resolver provider inicial e modelo default
7. criar `MessageBus`
8. criar `AgentLoop`
9. iniciar servicos auxiliares e canais
10. registrar callback de reload
11. iniciar o loop do agente e aguardar sinais
12. desligar servicos com timeout em encerramento ou reload

Esse startup concentra varias decisoes operacionais: validacao, fallback, observabilidade, hot reload e controle de ciclo de vida.

### 5.2 Fluxo do AgentLoop

`pkg/agent/loop.go` e o coracao do comportamento do sistema. O `AgentLoop` concentra:

- `EventBus` interno
- `HookManager`
- `ContextManager`
- fallback chain de providers
- runtime de MCP
- steering queue
- sincronizacao de turns ativos

Em alto nivel, o fluxo por mensagem e:

1. receber mensagem inbound do `MessageBus`
2. resolver metadados de sessao, canal, chat e sender
3. anexar media, voz, skills forçadas ou steering pendente
4. carregar contexto e historico respeitando budget
5. chamar o modelo atual
6. interpretar tool calls
7. executar tools, publicar feedback quando cabivel e retornar resultados ao contexto
8. repetir ate resposta final ou `max_tool_iterations`
9. persistir sessao e possivel sumario
10. publicar a resposta de saida para o canal correto

### 5.3 Steering e SubTurn

Dois subsistemas tornam o agente mais do que um simples loop request/response:

- `pkg/agent/steering.go`: permite injetar novas mensagens do usuario entre iteracoes de tool use
- `pkg/agent/subturn.go`: permite criar execucoes filhas com limites de profundidade, concorrencia, timeout e budget

Esses dois componentes sao importantes para reproduzir o comportamento observado do projeto, mas nao devem ser copiados literalmente em uma recriacao clean room. O contrato relevante e o efeito externo: permitir interrupcao/redirecionamento e trabalho em subagentes com isolamento controlado.

### 5.4 Hooks e EventBus

O sistema de hooks fica em `pkg/agent/hooks.go`, `pkg/agent/hook_process.go` e `pkg/agent/hook_mount.go`. Os documentos detalhados vivem em `docs/hooks/`.

Ele existe para:

- observacao de eventos do agente
- interceptacao antes/depois de chamadas LLM
- interceptacao antes/depois de tool calls
- gates de aprovacao humana
- extensibilidade via processos externos

O `EventBus` interno, em `pkg/agent/eventbus.go`, organiza notificacoes locais do loop.

## 6. Mapa dos Pacotes em `pkg/`

### 6.1 Nucleo do runtime

| Pacote | Papel |
| --- | --- |
| `pkg/agent` | loop principal, contexto, steering, subturn, hooks, eventbus, memoria do agente, integracao com tools e MCP |
| `pkg/gateway` | lifecycle do runtime: startup, reload, shutdown, integracao entre provider, channels e servicos |
| `pkg/config` | schema central, defaults, migration de versao, leitura por env, filtros de dados sensiveis, configuracao de agentes, canais e tools |
| `pkg/providers` | interface LLM, providers concretos, factory, cooldown, fallback, classificacao de erro, rate limiting |
| `pkg/channels` | interface de canal, base types, manager e implementacoes por plataforma |
| `pkg/bus` | pub/sub simples entre adaptadores de canal e loop do agente |
| `pkg/tools` | registro, schema e execucao das ferramentas do agente |
| `pkg/routing` | roteamento de modelo e resolucao de chaves/sessoes para binds e dispatch |

### 6.2 Persistencia, memoria e contexto

| Pacote | Papel |
| --- | --- |
| `pkg/session` | gerencia sessoes em memoria e persistencia em JSON por sessao |
| `pkg/memory` | `JSONLStore` append-only com truncacao logica, locks shardizados e recuperacao tolerante a linhas corrompidas |
| `pkg/seahorse` | armazenamento SQLite com FTS5 e trigram, mensagens estruturadas, sumarios, contexto compacto e busca textual |
| `pkg/media` | armazenamento e referencia de anexos e blobs de media |
| `pkg/state` | estado persistente auxiliar do runtime |

### 6.3 Servicos e operacao

| Pacote | Papel |
| --- | --- |
| `pkg/cron` | agendamento de tarefas com parser de cron |
| `pkg/heartbeat` | pings periodicos e tarefas de health |
| `pkg/health` | servidor HTTP de health e endpoints de monitoracao |
| `pkg/updater` | self-update e integracao com releases |
| `pkg/pid` | controle de PID file e singleton do gateway |
| `pkg/logger` | logging estruturado e integracao com arquivo/console |
| `pkg/isolation` | isolamento de subprocessos, principalmente para Linux |
| `pkg/devices` | device monitoring e integracoes de baixo nivel |

### 6.4 Extensibilidade e integracao

| Pacote | Papel |
| --- | --- |
| `pkg/mcp` | manager e integracao com servidores MCP |
| `pkg/skills` | carga, registro, instalacao, cache e busca de skills |
| `pkg/commands` | infraestrutura de slash commands e registro de comandos builtin |
| `pkg/audio` | ASR e TTS |
| `pkg/auth` | fluxos de autenticacao e artefatos ligados a providers |
| `pkg/credential` | armazenamento e suporte de credenciais |
| `pkg/identity` | resolucao de identidade de usuario ou sessao |
| `pkg/fileutil` | operacoes atomicas de arquivo usadas por persistencia |
| `pkg/utils` | helpers diversos de HTTP, markdown, strings, contextos e feedback |

## 7. Canais, Providers e Ferramentas

### 7.1 Canais suportados

Pelos imports do gateway, pela configuracao de exemplo e pela documentacao em `docs/channels/`, o projeto contem suporte ou scaffolding para varios canais, incluindo:

- Telegram
- Discord
- Slack
- Feishu
- DingTalk
- Weixin
- WeCom
- Line
- QQ
- OneBot
- Matrix
- IRC
- VK
- WhatsApp
- WhatsApp Native
- Pico
- Teams Webhook
- MaiXCam

Cada canal fica em `pkg/channels/<canal>/` e muitos possuem README proprio em `docs/channels/<canal>/`, inclusive em pt-BR.

### 7.2 Providers suportados

`pkg/providers/` contem o sistema de abstracao para modelos. O repositorio mostra suporte ou adaptadores para:

- OpenAI compativel
- Claude / Anthropic
- Azure OpenAI
- AWS Bedrock
- Gemini
- GitHub Copilot
- Codex e wrappers CLI
- Antigravity
- outros providers compatibilizados via model refs e factory

O contrato importante nao e o nome do arquivo de cada provider, e sim:

- como o provider e instanciado a partir de `model_list`
- como credenciais e `api_base` sao resolvidas
- como o fallback e o cooldown sao aplicados
- como erros sao classificados para retries, fallback ou falha final

### 7.3 Ferramentas builtin

`pkg/tools/` e uma das areas mais densas do repositorio. Ha ferramentas para:

- shell
- cron
- filesystem
- web search e web fetch
- MCP tools
- load_image
- session operations
- message e reaction
- spawn e spawn_status
- skills_search e skills_install
- send_file
- validacao e utilitarios diversos
- interfaces de hardware como I2C e SPI

As tools sao registradas pelo `AgentLoop` de acordo com a configuracao. Isso significa que paridade funcional depende tanto do codigo quanto dos toggles em `config.json`.

## 8. Launcher Web em `web/`

### 8.1 Estrutura

`web/` e um mini-monorepo:

- `web/backend/`: backend Go do launcher
- `web/frontend/`: frontend React 19 + TypeScript + Vite

### 8.2 Papel do backend

O backend do launcher faz mais do que servir HTML. Ele:

- controla a autenticacao do dashboard
- resolve o caminho de configuracao
- garante onboarding inicial
- sobe ou acopla o gateway
- expõe APIs REST para estado, configuracao e credenciais
- publica logs e status do subprocesso
- faz proxy de WebSocket para o canal Pico

### 8.3 Papel do frontend

Pelo `web/frontend/package.json`, o frontend usa:

- React 19
- TanStack Router
- TanStack Query
- i18next
- Tailwind CSS 4
- Radix UI e bibliotecas auxiliares

As capacidades principais do dashboard, segundo `web/README.md`, incluem:

- chat UI
- configuracao de modelos
- gerenciamento de credenciais
- configuracao de canais
- browser de skills
- visibilidade de tools
- ajustes de configuracao do agente e do launcher
- leitura de logs

### 8.4 Testes do launcher

O backend web tem cobertura consideravel em `web/backend/`, com testes para:

- middleware de auth e access control
- APIs de models, config, gateway, session, startup e auth
- launcher config
- embedding e onboard

Nao ha, pelo inventario do repositorio, uma suite de testes automatizados equivalente para o frontend React. Esse e um gap de QA real.

## 9. Configuracao, Dados e Seguranca

### 9.1 Fontes de configuracao

Arquivos e variaveis centrais:

- `config/config.example.json`: exemplo mais completo de schema operacional
- `PICOCLAW_CONFIG`: sobrescreve o caminho do `config.json`
- `PICOCLAW_HOME`: redefine o diretorio raiz de dados
- `PICOCLAW_LOG_LEVEL`: override do log level do gateway
- `PICOCLAW_LAUNCHER_TOKEN`: fixa o token do dashboard
- `PICOCLAW_BINARY`: permite ao launcher localizar explicitamente o binario principal
- `PICOCLAW_BUILTIN_SKILLS`: override do root de skills builtin

### 9.2 Layout do workspace

Segundo `docs/configuration.md` e o scaffold versionado em `workspace/`, o workspace esperado contem artefatos como:

- `AGENT.md`
- `USER.md`
- `SOUL.md`
- `memory/`
- `skills/`
- `sessions/`, `state/` e outros diretorios quando o runtime esta ativo

### 9.3 Persistencia de sessao e memoria

O repositorio contem mais de uma estrategia de persistencia:

- `pkg/session`: sessoes em memoria com persistencia em arquivos JSON
- `pkg/memory`: armazenamento append-only em JSONL com truncacao logica e tolerancia a corrupcao parcial
- `pkg/seahorse`: armazenamento SQLite com FTS5, sumarios e contexto compacto

Isso indica uma codebase em evolucao: a persistencia nao e um unico modulo monolitico.

### 9.4 Controles de seguranca observaveis

Ha varios mecanismos visiveis no codigo e na documentacao:

- filtragem de dados sensiveis no `pkg/config`
- autenticacao do launcher por token e cookie de sessao assinado
- opcionalmente `allowed_cidrs` no launcher em modo publico
- sanitizacao de nomes de sessao para persistencia cross-platform
- isolamento de subprocessos para tools em Linux
- documentacao especifica de criptografia de credenciais e dados sensiveis em `docs/credential_encryption.md` e `docs/sensitive_data_filtering.md`

Ao mesmo tempo, o proprio README alerta que o projeto ainda nao deve ser tratado como pronto para producao antes da v1.0.

## 10. Build, Teste, Release e Operacao

### 10.1 Build local

O `Makefile` da raiz oferece os fluxos principais:

- `make build`
- `make build-launcher`
- `make build-launcher-tui`
- `make build-all`
- `make build-linux-arm`, `build-linux-arm64`, `build-linux-mipsle`
- `make build-android-arm64`, `make build-android-bundle`
- `make build-whatsapp-native`

O foco do projeto em multiplataforma e real, nao decorativo: o Makefile trata MIPS, RISC-V, LoongArch, ARM, Windows, Darwin e Android.

### 10.2 Desenvolvimento do launcher web

`web/Makefile` organiza:

- `make dev`
- `make dev-frontend`
- `make dev-backend`
- `make build`
- `make build-frontend`
- `make test`
- `make lint`

O build do launcher embute os assets compilados do frontend em `web/backend/dist`.

### 10.3 Docker

`docker/docker-compose.yml` define tres perfis operacionais principais:

- `agent`: execucao one-shot do agente
- `gateway`: bot de longa duracao
- `launcher`: dashboard web + gateway

O compose monta `./data` em `/root/.picoclaw` e expoe a porta 18800 do launcher. Ha tambem Dockerfiles adicionais para goreleaser, imagem full e variacoes de build.

### 10.4 Release e CI

`.goreleaser.yaml` empacota:

- `picoclaw`
- `picoclaw-launcher`
- `picoclaw-launcher-tui`

Ha workflows em `.github/workflows/` para:

- build
- PR
- nightly
- release
- docker build
- criacao de DMG
- upload de artefatos

### 10.5 Testes e quality gates

O inventario do repositorio mostra cerca de 240 arquivos `*_test.go`. As areas com cobertura mais densa sao:

- `pkg/agent`
- `pkg/config`
- `pkg/providers`
- `pkg/channels`
- `pkg/tools`
- `pkg/session`
- `pkg/seahorse`
- `web/backend`

`.golangci.yaml` revela outro ponto importante de QA: o projeto usa `gofmt`, `gofumpt`, `goimports`, `gci` e `golines`, mas tambem desabilita um conjunto grande de linters. Isso mostra que a disciplina de formatacao esta consolidada, mas a barra de analise estatica ainda e permissiva em varias frentes.

## 11. Documentacao que Ja Existe

Antes de escrever nova documentacao, vale conhecer a que ja esta no repo:

- `docs/configuration.md` e `docs/pt-br/configuration.md`
- `docs/providers.md` e `docs/pt-br/providers.md`
- `docs/docker.md` e `docs/pt-br/docker.md`
- `docs/troubleshooting.md` e `docs/pt-br/troubleshooting.md`
- `docs/tools_configuration.md` e `docs/pt-br/tools_configuration.md`
- `docs/subturn.md`
- `docs/steering.md`
- `docs/design/steering-spec.md`
- `docs/hooks/README.md`
- `docs/hooks/hook-json-protocol.md`
- `docs/hooks/plugin-tool-injection.md`
- `docs/channels/*/README*.md`
- `docs/security_configuration.md`
- `docs/sensitive_data_filtering.md`
- `docs/credential_encryption.md`
- `docs/hardware-compatibility.md` e equivalente pt-BR

O que faltava, e que este documento tenta fechar, era a ponte entre todas essas pecas isoladas.

## 12. Lacunas Reais e Riscos da Codebase

### 12.1 Lacunas de documentacao

As lacunas mais importantes nao sao nos detalhes de canal; sao nos pontos de integracao:

- faltava uma visao unica de arquitetura do repositorio
- faltava um mapa claro de `pkg/` por responsabilidade
- faltava uma explicacao unica da separacao launcher/gateway
- faltava uma documentacao de persistencia comparando `session`, `memory` e `seahorse`
- faltava orientacao para leitura da codebase por quem quer reproduzir comportamento sem copiar implementacao

### 12.2 Riscos tecnicos a considerar

- o loop do agente e complexo e multi-responsabilidade; regressao aqui tem impacto sistêmico
- steering, subturn, hooks e tools convivem no mesmo eixo de execucao; bugs de concorrencia ou ordem de eventos tendem a ser sutis
- o launcher web depende da saude do subprocesso gateway; drift entre processos pode gerar bugs operacionais
- multiplataforma e objetivo central do projeto; alteracoes ingênuas em IO, paths ou subprocessos quebram rapidamente Windows, MIPS ou Android
- o repositorio tem forte cobertura em Go backend, mas nao mostra suite equivalente do frontend
- a configuracao e grande; compatibilidade reversa e migration merecem testes de contrato continuos

## 13. Guia para Clean Room Design

Se o objetivo e recriar funcionalidades sem copiar o codigo, use este repositorio como fonte de contratos observaveis, nao de estrutura interna.

### 13.1 O que deve ser tratado como contrato externo

- subcomandos CLI e seu papel funcional
- formato e semantica de `config.json`
- variaveis de ambiente publicas
- separacao entre launcher web e gateway
- comportamento do canal Pico e do exemplo `pico-echo-server`
- layout operacional do workspace
- existencia de sessions persistidas, historico, contexto resumido e tools habilitaveis por config
- comportamento de binds por canal, conta e peer
- hot reload, health e operacao basica do gateway

### 13.2 O que pode e deve ter identidade propria

- nomes de pacotes, structs e helpers
- composicao interna do loop do agente
- modelo de concorrencia, desde que mantenha garantias externas
- forma de persistencia interna, desde que o comportamento final seja equivalente
- desenho do frontend e UX do launcher, desde que os fluxos essenciais existam
- implementacao de providers e abstractions internas

### 13.3 Sinais de copia disfarçada que voce deve evitar

- repetir a mesma decomposicao de pastas e arquivos sem necessidade
- repetir a mesma sequencia exata de funcoes e helpers para startup e dispatch
- manter nomes de tipos, enums, flags internas e estruturas intermediarias apenas por similitude
- imitar um desenho de pacote que faz sentido so para a historia evolutiva deste repo
- importar a mesma fragmentacao de persistencia se voce puder consolidar melhor no seu projeto

### 13.4 Estrategia recomendada de reimplementacao

1. derive requisitos a partir de README, docs, exemplos, config de exemplo e comportamento observado
2. escreva testes de contrato seus antes de implementar detalhes internos
3. modele seus dominios com nomes proprios
4. trate o comportamento externo como alvo e o design interno como independente
5. valide paridade de comportamento por cenarios, nao por semelhanca estrutural

### 13.5 Checklist minimo de paridade funcional

- startup com configuracao ausente ou invalida
- onboarding e resolucao de paths
- gateway com modelo default valido e invalido
- launcher web conseguindo subir, autenticar e anexar ao gateway
- persistencia de sessao entre reinicios
- tools habilitadas e desabilitadas por config
- dispatch e resposta de pelo menos um canal externo e do canal Pico
- fallback entre modelos/providers
- slash commands basicos
- cron, health e logs operacionais

## 14. Ordem Recomendada de Leitura

### 14.1 Para operadores

1. `README.pt-br.md`
2. `docs/pt-br/configuration.md`
3. `docs/pt-br/docker.md`
4. `docs/pt-br/troubleshooting.md`
5. `web/README.md`

### 14.2 Para contribuidores Go

1. `cmd/picoclaw/main.go`
2. `pkg/gateway/gateway.go`
3. `pkg/agent/loop.go`
4. `pkg/config/config.go`
5. `pkg/providers/factory.go` e `pkg/providers/fallback.go`
6. `pkg/channels/manager.go`
7. testes em `pkg/agent`, `pkg/tools` e `pkg/channels`

### 14.3 Para quem vai reimplementar por clean room

1. este documento
2. `config/config.example.json`
3. `docs/pt-br/configuration.md`
4. `web/README.md`
5. `examples/pico-echo-server/README.md`
6. `docs/channels/*/README*.md` para os canais que voce realmente vai suportar
7. somente depois disso: leitura seletiva de codigo para confirmar efeitos observaveis

## 15. Conclusao

PicoClaw nao e um binario unico com alguns plugins. E uma plataforma relativamente ampla, com:

- runtime multicanal
- loop de agente com tools, hooks, steering e subturn
- multiplos backends de persistencia e contexto
- launcher web separado do gateway
- preocupacao real com multiplataforma e edge deployment

O entendimento correto do repositorio depende de enxergar essas camadas ao mesmo tempo. Se voce tentar ler apenas por ordem alfabetica de pasta, vai subestimar as fronteiras reais do sistema. Se o objetivo e QA, extensao ou clean room design, comece pelos contratos externos e pelos fluxos de runtime, nao pelos helpers internos.
