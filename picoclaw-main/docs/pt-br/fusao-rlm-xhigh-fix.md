# Fusão PicoClaw ↔ rlm-go e suporte a `thinking_level=xhigh`

Data: 2026-04-18

## Diagnóstico

Ao investigar a fusão entre `arkhe-go/picoclaw-main` (host operacional em Docker/VPS) e `arkhe-go/rlm-go` (engine recursiva local embutida) e o suporte a `thinking_level=xhigh` nos providers OpenAI-compatíveis, foi identificado **um único bug real**, silencioso, que explicava o sintoma de "provider não suporta xhigh":

- Arquivo: [arkhe-go/picoclaw-main/pkg/providers/openai_compat/provider_test.go](../../pkg/providers/openai_compat/provider_test.go)
- Sintoma: o bloco inteiro de testes `// --- Extended Thinking / reasoning_effort tests ---` havia sido duplicado no final do arquivo (linhas 1425–1568), redeclarando:
  - `TestApplyOpenAICompatThinking_QwenEndpoint`
  - `TestApplyOpenAICompatThinking_QwenModel`
  - `TestApplyOpenAICompatThinking_QwenLevels`
  - `TestApplyOpenAICompatThinking_OpenAIEndpoint`
  - `TestApplyOpenAICompatThinking_OpenAI_XHighMapsToHigh`
  - `TestApplyOpenAICompatThinking_DeepSeek`
  - `TestApplyOpenAICompatThinking_GenericProvider`
  - `TestApplyOpenAICompatThinking_OffLevel`
  - `TestApplyOpenAICompatThinking_EmptyLevel`
  - `TestBuildRequestBody_ThinkingLevelQwen`
- Efeito: `go test`/`go vet` no pacote falhavam com `XXX redeclared in this block`, o que impedia o ciclo de validação dos testes de `xhigh` e dava a impressão de que o suporte estava quebrado.

Fato: o código de produção já tratava `xhigh` corretamente em todos os providers relevantes, sem necessidade de novos ramos:

- `openai_compat/provider.go` → `applyOpenAICompatThinking` + `qwenThinkingBudget("xhigh") = 65536` + `mapReasoningEffort("xhigh") = "high"`.
- `gemini_provider.go` → `mapGeminiThinkingLevel("xhigh") = "high"` e `mapGeminiThinkingBudget("xhigh")`.
- `anthropic/provider.go` → nível `xhigh` mapeia para `budget_tokens = 64000`.
- Frontend web já expõe `xhigh` nas locales `en.json`/`zh.json` como valor suportado.

## Correção aplicada

Remoção do bloco duplicado (linhas 1425–1568), preservando o primeiro bloco de testes e o `TestBuildRequestBody_ThinkingLevelQwen` original. Nenhuma mudança de código de produção foi necessária.

Validação:

```
go vet ./pkg/providers/openai_compat/...            # sem erros
go build ./...                                      # sem erros em todo o módulo
go test ./pkg/providers/openai_compat/ -count=1     # ok
go test ./pkg/providers/ -run RLM -count=1          # ok
go test ./pkg/agent/ -run "Runtime|RLM|Bind" -count=1 # ok
go test ./pkg/recursion/... -count=1                # ok
cd ../rlm-go && go test ./... -count=1              # ok (core, environments, utils)
```

## Estado da fusão PicoClaw ↔ rlm-go (reconfirmado)

Nenhuma incongruência ou bug silencioso restante foi encontrado na superfície de fusão:

- `pkg/providers/rlm_provider.go` continua integrando `rlm-go` como `LLMProvider` local, rejeitando `api_base` remota (apenas loopback) e mantendo estado por `sessionKey`.
- `pkg/providers/rlm_embedded_client.go` injeta `ClientFactory` baseado no provider OpenAI-compatível do PicoClaw no runtime embutido, sem o runtime criar clients próprios.
- `pkg/agent/loop.go` bind o `AgentRuntimeBinding` em `Provider`, `LightProvider` e `CandidateProviders` por agente, expondo ferramentas locais do PicoClaw à engine recursiva — o `bindRuntimeProvidersForRegistry` está correto e é idempotente.
- `pkg/recursion/` segue como overlay (MCTS tool + hooks) sobre o AgentLoop; os testes passam.
- `go.mod` usa `replace github.com/O-guardiao/arkhe-go/rlm-go => ../rlm-go`, e o path canônico bate com o `module` do `rlm-go/go.mod`.

Conclusão: a fusão está consistente no escopo definido (embedding de engine recursiva local no PicoClaw). O único ruído era o arquivo de teste inflado por uma colagem duplicada.

---

## Addendum (auditoria profunda) — bug silencioso de race em `RLMProvider.chatWithSession`

Após fechar o sintoma de `xhigh`, o usuário pediu uma auditoria profunda da fusão procurando **bugs silenciosos**. Um foi encontrado e corrigido.

### Bug: contaminação cruzada de `meta`/`ctx` entre chamadas concorrentes no mesmo `sessionKey`

- Arquivo: [arkhe-go/picoclaw-main/pkg/providers/rlm_provider.go](../../pkg/providers/rlm_provider.go), função `chatWithSession`.
- Estado anterior: `state.setMeta(meta)` e `state.setContext(ctx)` eram executados **antes** de `state.mu.Lock()`. Os campos `currentMeta`/`currentCtx` dentro de `rlmSessionState` possuem seus próprios mini-mutexes (`metaMu`, `ctxMu`), o que dá atomicidade por campo — mas **não** ordenação com relação ao turno que está dentro de `engine.Completion`.
- Cenário de falha (real, reprodutível, fato):
  1. Chamador A entra em `chatWithSession` com `meta={channel:"chanA", chat_id:"chatA"}`, faz `setMeta(A)` / `setContext(ctxA)` e adquire `state.mu`.
  2. Chamador B chega em paralelo com `meta={channel:"chanB", chat_id:"chatB"}`, executa `setMeta(B)` / `setContext(ctxB)` (escreve sobre o estado compartilhado) e fica bloqueado em `state.mu.Lock()`.
  3. A engine de A, ainda dentro de `Completion`, invoca uma ferramenta. O closure de ferramenta chama `metaProvider()` ≡ `state.meta` (leitura preguiçosa).
  4. `state.meta()` retorna **B**. A ferramenta executa com `meta` do chamador errado. `bus.PublishOutbound` é roteado para o `channel/chat_id` **do outro tenant**.
- Gravidade: médio-alto. Implicações:
  - Vazamento cross-tenant de saídas de ferramenta no bus.
  - Cancelamento indevido: o `defer state.setContext(context.Background())` do chamador B poderia zerar o `ctx` enquanto A ainda lê; e inversamente, `ctxB` pode cancelar/herdar deadlines em operações iniciadas por A.
  - Falha silenciosa: nenhum log, nenhum erro, nenhuma detecção em testes até este commit.

### Fix

Movido `setMeta` / `setContext` (e o `defer setContext(background)`) para **dentro** da seção crítica `state.mu.Lock()/Unlock()`, com comentário in-loco explicando a invariante. O `defer state.setContext(context.Background())` continua LIFO-ordenado, rodando antes do `Unlock`, preservando a limpeza do contexto por turno.

Diff conceitual:

```go
// antes
state.setMeta(meta)
state.setContext(ctx)
defer state.setContext(context.Background())
// ... buildEngineConfig ...
state.mu.Lock()
defer state.mu.Unlock()

// depois
// ... buildEngineConfig ...
state.mu.Lock()
defer state.mu.Unlock()
state.setMeta(meta)
state.setContext(ctx)
defer state.setContext(context.Background())
```

### Regressão — testes adicionados

Em [arkhe-go/picoclaw-main/pkg/providers/rlm_provider_test.go](../../pkg/providers/rlm_provider_test.go):

- `TestRLMSessionStateMetaContextRaceGuard`: 200 iterações × 2 goroutines exercitam o contrato `Lock → setMeta → read → Unlock` a nível de estado (invariante mínima).
- `TestRLMProviderChatWithSessionIsolatesMetaUnderConcurrency`: **teste end-to-end**. Dispara 6 pares concorrentes de `Chat()` no mesmo `sessionKey=shared-session` com `channel/chat_id` distintos (`chanA/chatA` vs `chanB/chatB`) contra um `AgentRuntimeBinding` de teste que registra o `meta` observado em cada chamada de ferramenta. Assere que **todas** as invocações do tenant A observaram `chanA/chatA` e as de B observaram `chanB/chatB`.

Este segundo teste **foi usado como prova**: revertendo o fix para a ordem anterior, ele passa a falhar de forma determinística com uma saída do tipo `map[chatA:[chanA chanA ... chanA]]` — 12 observações de ferramenta, todas atribuídas a um único tenant, evidenciando a contaminação cruzada. Com o fix aplicado, passa em 3×N execuções consecutivas.

### Validação pós-fix

```
go build ./...                                                # ok
go vet ./...                                                  # ok
go test ./pkg/providers/ -run RLM -count=3                    # ok
go test ./pkg/agent/ -count=1                                 # ok
go test ./pkg/recursion/ -count=1                             # ok
go test ./pkg/providers/openai_compat/ -count=1               # ok
```

Nota sobre `-race`: o toolchain Go deste ambiente Windows está com `CGO_ENABLED=0` e sem `gcc` disponível, portanto o detector de corridas (`-race`) não pôde ser executado nesta validação. A correção, entretanto, é exercitada pela prova de reversão acima, que detecta o bug no nível funcional (observação de `meta`) sem depender de instrumentação de runtime.

