# Operacao, Build e Release

## Escopo

Este documento cobre os ativos que sustentam o ciclo de vida do projeto, nao apenas a execucao do agente:

- arquivos raiz de build e policy
- `docker/`
- `.github/`
- `scripts/`
- `config/`
- `examples/`
- `workspace/`
- `docs/`

## 1. Build local

### `Makefile` da raiz

Esse arquivo e o principal contrato de desenvolvimento local. Entre os alvos mais relevantes:

- `make generate`
- `make build`
- `make build-launcher`
- `make build-launcher-frontend`
- `make build-launcher-tui`
- `make build-all`
- `make build-linux-arm`
- `make build-linux-arm64`
- `make build-linux-mipsle`
- `make build-android-arm64`
- `make build-android-bundle`
- `make build-pi-zero`
- `make build-whatsapp-native`

Leitura tecnica:

- o projeto se preocupa seriamente com multiplataforma
- parte do build e sensivel a tags de compilacao (`goolm`, `stdjson`, `whatsapp_native`)
- ha ajustes especificos para MIPS, Android, macOS e multiplos targets edge

## 2. Dependencias e shape tecnologico

### `go.mod`

O arquivo mostra um ecossistema de dependencias que confirma a natureza do produto:

- SDKs de canais sociais e enterprise
- SDKs de multiplos providers LLM
- `cobra` para CLI
- `zerolog` para logging
- `modernc.org/sqlite` para persistencia Seahorse
- stack WebSocket, OAuth2 e Model Context Protocol

Isso reforca a leitura de que PicoClaw e um runtime de agente multicanal, nao apenas uma CLI com alguns wrappers.

## 3. Docker

### `docker/docker-compose.yml`

Perfis observados:

- `picoclaw-agent`: one-shot
- `picoclaw-gateway`: daemon de longa duracao
- `picoclaw-launcher`: console web + gateway

### Dockerfiles

- `Dockerfile`: imagem principal
- `Dockerfile.full`: variante ampliada
- `Dockerfile.goreleaser`: pipeline de release
- `Dockerfile.goreleaser.launcher`: release do launcher
- `Dockerfile.heavy`: variante adicional

### Contrato operacional

- o sistema precisa rodar de forma suportada em container
- o launcher precisa conseguir servir UI e controlar gateway nesse contexto
- o volume de dados em `/root/.picoclaw` faz parte do contrato de persistencia operacional

## 4. GitHub Actions e release

### `.github/workflows/`

Workflows observados:

- `build.yml`
- `pr.yml`
- `release.yml`
- `nightly.yml`
- `docker-build.yml`
- `create_dmg.yml`
- `upload-tos.yml`

Leitura pratica:

- ha pipeline de build em PR
- ha releases formais e nightly
- ha empacotamento de imagens Docker
- ha suporte a artefatos de desktop, incluindo DMG

### `.goreleaser.yaml`

Empacota:

- `picoclaw`
- `picoclaw-launcher`
- `picoclaw-launcher-tui`

Tambem trata:

- multiplataforma agressiva
- imagens Docker
- pacotes RPM/DEB
- notarizacao macOS quando secrets existem

## 5. Scripts e suporte a empacotamento

`scripts/` contem artefatos pontuais, mas importantes:

- `build-macos-app.sh`
- `setup.iss`
- `icon.icns`
- `test-docker-mcp.sh`
- `test-irc.sh`

Esses arquivos mostram um projeto com preocupacao real de distribuicao e smoke tests de integracao, mesmo que nem tudo esteja formalizado em CI end-to-end.

## 6. Config de referencia e exemplos

### `config/config.example.json`

Esse e o documento mais importante para reimplementacao a partir de comportamento. Ele expõe:

- schema pratico de `config.json`
- exemplos de `model_list`
- configuracao de canais
- toggles de tools
- defaults do agente

### `examples/pico-echo-server/`

Esse exemplo e valioso porque documenta o protocolo Pico do ponto de vista observavel.

## 7. Workspace template

`workspace/` nao e apenas exemplo. Ele expressa a concepcao de produto do runtime local:

- arquivos de identidade do agente
- memoria local
- skills instaladas localmente

Isso torna `workspace/` uma parte conceitual do produto, nao apenas um fixture.

## 8. Documentacao existente

### Documentos tecnicos nucleares

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

### Guias por canal

`docs/channels/` contem documentacao dedicada para varios canais, geralmente em multiplos idiomas.

### Traducoes

Ha espelhamento importante em `pt-br`, `zh`, `ja`, `vi`, `fr`, `my`. Para engenharia, isso e secundario. Para produto e adocao, isso e relevante.

## 9. Gaps operacionais e documentais

- falta uma matriz unica de build x target x feature support
- falta uma documentacao central de release flow para mantenedores
- falta uma especificacao compacta do contrato do workspace local
- falta um mapa unico de variaveis de ambiente e precedencia
- falta um documento unificado de smoke tests operacionais

## 10. O que preservar numa reimplementacao clean room

- facilidade de setup local
- multiplataforma como objetivo real, nao acessorio
- launcher web e runtime containerizados
- configuracao orientada a arquivo + env overrides
- workspace local como parte do produto

## 11. O que pode ser redesenhado

- pipeline de release e tooling
- escolha exata de build scripts
- estrutura de imagens e compose
- forma como exemplos e workspace template sao organizados
