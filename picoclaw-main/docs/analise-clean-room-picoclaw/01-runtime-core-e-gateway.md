# 01 Runtime Core e Gateway

## Tese Estrutural

O centro funcional de PicoClaw nao e a CLI em si, mas um runtime persistente iniciado pelo gateway. A CLI so decide qual superficie entra em cena; o comportamento real nasce quando o gateway monta:

1. config carregada e validada;
2. provider inicial e fallback chain;
3. `MessageBus`;
4. `AgentLoop`;
5. canais, media store, heartbeat, cron, health e device services.

## Fluxo Principal de Execucao

### Boot

- `cmd/picoclaw/main.go` encaminha para subcomandos.
- `pkg/gateway/gateway.go` faz `Run()` e vira orquestrador de processo.
- `config.LoadConfig()` define defaults, segredos resolvidos e schema v2.
- `agent.NewAgentLoop()` constroi registry, fallback, context manager, hooks e tools compartilhadas.
- `channels.NewManager()` sobe adapters e acopla HTTP/WS quando necessario.

### Turno de mensagem

1. Canal publica `InboundMessage` no bus.
2. `AgentLoop.Run()` escolhe agente, contexto, ferramentas e modelo.
3. Slash commands passam por `pkg/commands` antes de cair no LLM.
4. Tool calls entram em iteracao controlada por limite de passos.
5. Resposta final volta para o bus como `OutboundMessage`.
6. Sessao e estado local sao persistidos.

### Reload e operacao

- `pkg/health/server.go` expoe `/health`, `/ready`, `/reload`.
- O gateway pode recarregar config sem reiniciar todo o processo.
- `pid` e `health` sustentam tanto o launcher web quanto a observabilidade local.

## Pastas e Arquivos-Chave

| Arquivo/Pasta | Papel |
| --- | --- |
| `cmd/picoclaw/main.go` | Entrypoint da CLI |
| `pkg/gateway/gateway.go` | Startup, reload, shutdown e wiring de servicos |
| `pkg/agent/loop.go` | Loop principal do agente |
| `pkg/agent/turn.go` | Fases do turno e estado de iteracao |
| `pkg/agent/subturn.go` | Turnos aninhados e spawn de subagentes |
| `pkg/agent/events.go` | Event bus semantico do runtime |
| `pkg/bus/bus.go` | Barramento de entrada/saida/media/voice |
| `pkg/session/manager.go` | Historico de sessao e persistencia atomica |
| `pkg/state/state.go` | Ultimo canal/chat e contexto operacional minimo |
| `pkg/health/server.go` | Prontidao, health e reload |

## Contratos de Paridade que Devem Sobreviver

### 1. Conversa persistente por sessao

Uma reimplementacao precisa preservar:

- chave de sessao estavel por conversa/canal;
- historico reaproveitado em turnos seguintes;
- truncamento/summary como mecanismo de controle de contexto;
- persistencia append-only ou semanticamente equivalente.

### 2. Orquestracao multi-servico

O gateway precisa continuar sendo um coordenador de servicos, nao apenas um wrapper para um prompt unico. O comportamento esperado inclui:

- subir canais e tools sob a mesma configuracao;
- manter runtime vivo entre mensagens;
- suportar health, restart e reload;
- fechar recursos de forma previsivel no shutdown.

### 3. Eventing interno

Mesmo que voce troque `EventBus` por outro modelo, preserve:

- eventos de inicio/fim de turno;
- eventos de tool execution;
- rastreabilidade minima para debug e UI.

## Validacao Clean Room

### O que seria copia disfarçada

- repetir structs e enums com os mesmos nomes e fases (`turnState`, `EventKind`, `FallbackResult`);
- reproduzir a mesma ordem de inicializacao do gateway linha por linha;
- manter a mesma divisao entre `loop.go`, `turn.go`, `events.go`, `subturn.go` sem justificativa propria;
- espelhar a mesma estrategia de registry e wiring por simples transliteracao.

### O que e reimplementacao saudavel

- reorganizar o motor em `runtime`, `conversation`, `delivery`, `orchestration` ou outro desenho proprio;
- trocar `AgentLoop` por pipeline com stages mais explicitos;
- usar envelopes e nomes diferentes para sessao, iteracao e subexecucao;
- separar melhor o que e transporte, memoria e motor de inferencia.

## Edge Cases e Hotspots

- sessao com arquivo corrompido ou parcial;
- reload com config valida, mas provider indisponivel;
- shutdown enquanto tools ou streaming ainda estao em andamento;
- race entre restart do gateway e proxys/websocket do launcher;
- crescimento de historico sem summary consistente;
- subturns concorrentes publicando na mesma sessao.

## Sugestoes de Teste Essenciais

1. Reaproveitamento de historico: duas mensagens na mesma sessao devem manter memoria e ordem de eventos.
2. Reload seguro: troca de config deve manter o processo vivo e refletir novo provider/canal.
3. Shutdown gracioso: parada durante tool longa nao deve corromper sessao nem deixar worker preso.
4. Subturn concorrente: duas execucoes filhas devem retornar sem perder correlacao de sessao.
5. Recuperacao de sessao: arquivo persistido deve sobreviver a restart e manter transcript valido.
