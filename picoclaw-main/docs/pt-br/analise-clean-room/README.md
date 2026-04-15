# Suite de Analise Clean Room do PicoClaw

Esta pasta organiza uma leitura tecnica do repositorio com foco em tres objetivos:

- engenharia reversa por comportamento, sem copia estrutural
- QA de arquitetura, risco e superficie de regressao
- navegacao rapida por um repositorio grande, heterogeneo e multiplataforma

Esta suite complementa o documento macro em `docs/pt-br/documentacao-completa-do-repositorio.md`. Aqui o foco nao e apenas explicar o projeto; e separar contratos observaveis, hotspots de risco, fronteiras entre modulos e pontos onde uma recriacao clean room precisa tomar decisoes proprias.

## Como usar

Ordem recomendada:

1. `00-inventario-estrutural.md`
2. `01-runtime-core.md`
3. `02-integracoes-externas.md`
4. `03-launchers-e-superficies.md`
5. `04-operacao-build-release.md`
6. `05-guia-de-qa-e-clean-room.md`

## O que esta coberto

- mapa do repositorio com contagem de arquivos por area
- runtime core em `cmd/` e `pkg/`
- canais, providers, tools, MCP, audio, auth e isolamento
- launcher web, dashboard React e launcher TUI
- pipeline de build, Docker, GitHub Actions, release e workspace template
- riscos de seguranca, edge cases, concorrencia e testabilidade
- diretrizes praticas para reimplementacao clean room

## O que nao foi tratado como prioridade

- assets de imagem analisados individualmente
- todas as traducoes repetidas em `docs/` arquivo por arquivo
- cada arquivo de teste descrito isoladamente quando a familia de arquivos tem comportamento repetitivo

Isso foi intencional. Para clean room design, o valor tecnico esta nas superficies executaveis, nos contratos de configuracao, nos fluxos de runtime e nas integracoes. Os ativos puramente editoriais ou repetitivos entram por agrupamento.

## Estrategia usada

- varredura estrutural do repositorio inteiro
- leitura distribuida por subagentes em quatro frentes: core, integracoes, launchers e operacao
- consolidacao dos contratos observaveis e riscos de QA
- documentacao local em portugues, orientada a engenharia

## Proximo uso esperado

Quando voce enviar um modulo da sua reimplementacao, a avaliacao deve ser feita contra esta suite e contra o comportamento do produto original, usando o formato de revisao definido em `05-guia-de-qa-e-clean-room.md`.
