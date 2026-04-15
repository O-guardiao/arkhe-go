# Analise Clean Room do picoclaw-main

Esta suite organiza uma leitura tecnica do repositiorio picoclaw-main com foco em engenharia reversa por comportamento, QA arquitetural, riscos de seguranca e preparacao para reimplementacao clean room.

## Conclusoes Centrais

- O produto canonico vive em tres massas principais: `pkg/` para runtime e integracoes, `cmd/` para superficies executaveis e `web/` para launcher/dashboard.
- `pkg/agent`, `pkg/gateway`, `pkg/channels`, `pkg/providers` e `pkg/tools` formam o contrato comportamental mais importante; todo o resto orbita essas camadas.
- O repositorio nao e apenas um CLI. Ele combina loop agente persistente, multiplexacao de canais, fallback entre LLMs, pipeline de ferramentas, launcher web, persistencia local e recarga operacional.
- A maior parte do risco clean room nao esta em copiar algoritmos complexos isolados, mas em copiar nomenclatura, decomposicao de pastas, ordem de helpers e o mesmo encadeamento de startup.
- O principal risco de seguranca confirmado e a combinacao de `exec` habilitado/remoto por default com canais que aceitam qualquer remetente quando `allow_from` fica vazio.

## Ordem Recomendada de Leitura

1. `00-inventario-estrutural.md`
2. `01-runtime-core-e-gateway.md`
3. `02-canais-providers-tools.md`
4. `03-launcher-web-e-operacao.md`
5. `04-build-config-docs-workspace.md`
6. `05-guia-qa-clean-room.md`
7. `06-matriz-de-testes.md`
8. `99-lista-de-arquivos-prioritarios.md`

## O que esta suite entrega

- mapa estrutural por pasta e subsistema;
- separacao entre codigo canonico, superficies operacionais, docs e ativos auxiliares;
- contratos observaveis que uma reimplementacao precisa preservar;
- achados de QA, edge cases, seguranca e operacao;
- lista de hotspots onde uma copia disfarçada e mais provavel;
- matriz de testes para validar paridade sem depender da forma do codigo original.

## Nota de Clean Room

Uma reimplementacao saudavel de PicoClaw nao deve copiar mecanicamente:

- os mesmos nomes de structs como `AgentLoop`, `FallbackChain`, `BaseChannel` ou `Handler`;
- a mesma decomposicao em `pkg/channels`, `pkg/providers`, `pkg/tools` com subpastas identicas apenas por inercia;
- a mesma ordem de inicializacao de servicos e registries;
- o mesmo fluxo literal de CLI -> gateway -> agent loop -> channels -> tools.

O que deve ser preservado e:

- o comportamento observavel do runtime;
- os contratos entre canais, modelos, ferramentas e sessao;
- as garantias operacionais de configuracao, startup, reload e persistencia;
- os limites de seguranca e as formas de degradacao quando algo falha.

## Resultado Pratico

Se o objetivo for recriar o produto por comportamento, comece por:

1. loop de mensagem, sessao e ferramentas;
2. multiplexacao de canais e fallback entre provedores;
3. launcher web e proxy Pico;
4. migracao/configuracao e ergonomia de operacao.

Se o objetivo for auditar riscos antes de reimplementar, va direto para `05-guia-qa-clean-room.md` e `06-matriz-de-testes.md`.
