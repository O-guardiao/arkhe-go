# RLM Go

Reconstrucao em Go do runtime observado em `rlm-main-original`, guiada pela analise clean-room em `docs/analise-clean-room-rlm`.

## O que esta implementado

- Loop iterativo `RLM.Completion()`
- `LMHandler` TCP com protocolo `4-byte length prefix + JSON`
- `LocalREPL` persistente em Go usando interpretador embutido
- `llm_query`, `llm_query_batched`, `rlm_query`, `rlm_query_batched`
- `context`, `context_N`, `history`, `history_N`
- `FINAL(...)` e `FINAL_VAR(...)`
- Persistencia multi-turno para ambiente `local`
- Compaction do historico raiz
- Logger de trajetoria em memoria e JSONL
- Clientes `openai`, `vllm`, `openrouter`, `vercel`, `portkey`, `azure_openai`, `anthropic`, `gemini`
- Backend `mock` para testes

## Estrutura

- `core/`: loop recursivo, handler TCP, limites, subcalls
- `environments/`: ambiente local persistente e contratos de ambiente
- `clients/`: clientes LM e factory de backends
- `protocol/`: wire protocol compartilhado entre runtime e REPL
- `logger/`: captura de trajetoria
- `utils/`: prompts, parsing, token utils, erros
- `types/`: tipos publicos do runtime

## Diferenca importante

O ambiente de referencia do projeto original e o `local`, e esta porta implementa esse caminho completo em Go. Os adapters remotos (`docker`, `modal`, `prime`, `daytona`, `e2b`) permanecem como pontos de extensao explicitos e hoje retornam erro de nao implementado.

## Quickstart

```go
package main

import (
	"fmt"
	"log"

	rlm "github.com/O-guardiao/arkhe-go/rlm-go"
)

func main() {
	engine, err := rlm.New(rlm.Config{
		Backend: "openai",
		BackendKwargs: map[string]any{
			"model_name": "gpt-4o-mini",
		},
		MaxDepth: 1,
	})
	if err != nil {
		log.Fatal(err)
	}

	result, err := engine.Completion("Liste os primeiros 10 numeros primos.", "")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Response)
}
```

## Testes

Em ambientes sandboxados, mantenha cache do Go dentro do workspace:

```powershell
$env:GOMODCACHE="$PWD\\.gomodcache"
$env:GOCACHE="$PWD\\.gocache"
go test ./...
```

## Estado da porta

Essa porta cobre o comportamento central descrito na analise clean-room e validado pelos testes locais do modulo Go. O foco foi fidelidade funcional do caminho de referencia: runtime recursivo, estado persistente, I/O local, parsing final, compaction e protocolo de subchamadas.
