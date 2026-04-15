# Integracoes Externas

## Escopo

Este documento cobre os subsistemas que ligam o runtime a plataformas, LLMs, tools, audio, autenticacao e isolamento:

- `pkg/channels`
- `pkg/providers`
- `pkg/tools`
- `pkg/mcp`
- `pkg/audio`
- `pkg/media`
- `pkg/auth`
- `pkg/credential`
- `pkg/devices`
- `pkg/identity`
- `pkg/isolation`

## 1. Canais como fronteira de plataforma

### Arquitetura geral

`pkg/channels/` e a maior area do repositorio. Isso ja diz muito sobre o produto: o sistema foi desenhado para ser multicanal de verdade, nao apenas um chatbot com um adaptador adicional.

Arquivos estruturais:

- `base.go`: base comum e utilitarios compartilhados
- `interfaces.go`: contratos dos canais
- `manager.go`, `manager_channel.go`: dispatcher central
- `registry.go`: descoberta/registro
- `dynamic_mux.go`: multiplexacao dinamica
- `media.go`, `webhook.go`, `voice_capabilities.go`: suportes transversais
- `errors.go`, `errutil.go`, `split.go`, `marker.go`: utilitarios de envio e erro

### Subpastas de canal observadas

- `telegram/`
- `discord/`
- `slack/`
- `weixin/`
- `wecom/`
- `feishu/`
- `dingtalk/`
- `line/`
- `qq/`
- `onebot/`
- `matrix/`
- `irc/`
- `pico/`
- `vk/`
- `maixcam/`
- `whatsapp/`
- `whatsapp_native/`
- `teams_webhook/`

### Padrao de implementacao

Cada canal tende a combinar:

- arquivo principal `<canal>.go`
- `init.go` para registro
- possiveis auxiliares de protocolo, auth, media, voz ou cache
- `*_test.go` focados no contrato daquele canal

Exemplos relevantes:

- `telegram/command_registration.go`: registro de comandos na plataforma
- `telegram/parser_markdown_to_html.go` e `parse_markdown_to_md_v2.go`: camada de formatacao especifica
- `discord/voice.go`: integracao de voz
- `weixin/api.go`, `auth.go`, `media.go`, `state.go`, `types.go`: canal mais acoplado a protocolo proprio
- `wecom/protocol.go`, `reqid_store.go`, `media.go`: fluxo mais complexo de protocolo e upload
- `pico/protocol.go`, `client.go`, `pico.go`: protocolo interno/proprietario do ecossistema do projeto

### Contrato observavel para clean room

O contrato aqui nao e “ter as mesmas structs”. O contrato e:

- receber mensagens de varias plataformas
- normalizar para o runtime
- enviar respostas de volta respeitando limitacoes do canal
- lidar com identidade, allow-lists, formatos e media

### Principais riscos de QA nos canais

- diferencas de formato entre markdown, HTML e payload do canal
- limites de tamanho e splitting de mensagem
- idempotencia em webhooks e eventos repetidos
- vazamento de resposta para chat errado por erro de roteamento
- tratamento inconsistente de threads, replies e canais privados/publicos

## 2. Providers como fronteira com LLMs

### Estrutura principal de `pkg/providers/`

Arquivos estruturais:

- `types.go`: contratos de provider
- `factory.go`, `factory_provider.go`: resolucao e instanciacao
- `fallback.go`: cadeia de fallback
- `cooldown.go`: cooldown de erros e rate limits
- `ratelimiter.go`: limitacao de taxa
- `error_classifier.go`: classificacao de erro
- `model_ref.go`: resolucao de referencia de modelo
- `http_provider.go`: base HTTP
- `tool_call_extract.go`, `toolcall_utils.go`: parsing de tool calls

Implementacoes e adaptadores observados:

- `anthropic/`
- `anthropic_messages/`
- `azure/`
- `bedrock/`
- `openai_compat/`
- `openai_responses_common/`
- `gemini_provider.go`
- `github_copilot_provider.go`
- `codex_provider.go`
- `codex_cli_provider.go`
- `claude_provider.go`
- `claude_cli_provider.go`
- `antigravity_provider.go`

### Contrato observavel dos providers

- o runtime conversa com diferentes provedores por uma interface unificada
- alguns providers suportam streaming
- alguns suportam thinking / extended reasoning
- fallbacks e cooldown fazem parte do comportamento resiliente

### Principais riscos de QA nos providers

- classificacao de erro equivocada levando a fallback indevido
- diferencas de schema de tool calls entre providers
- bugs de compatibilidade entre APIs OpenAI-compatible e APIs nativas
- inconsistencias de modelo default versus `model_name` em config
- regressao em auth e base URLs customizadas

## 3. Tools como superficie de acao

### Estrutura principal de `pkg/tools/`

Arquivos centrais:

- `base.go`: contrato da tool
- `registry.go`: registro e descoberta
- `types.go`, `result.go`, `normalization.go`: contratos de execucao e resultados
- `toolloop.go`: apoio ao fluxo do agente

Ferramentas observadas por familia:

#### Arquivos e sistema

- `filesystem.go`
- `edit.go`
- `send_file.go`
- `load_image.go`

#### Processos e shell

- `shell.go`
- `shell_process_unix.go`, `shell_process_windows.go`
- `session.go`
- `session_process_unix.go`, `session_process_windows.go`
- `sysproc_unix.go`, `sysproc_windows.go`

#### Busca, web e skills

- `web.go`
- `search_tool.go`
- `skills_search.go`
- `skills_install.go`

#### Coordenacao do proprio agente

- `spawn.go`
- `spawn_status.go`
- `subagent.go`
- `message.go`
- `reaction.go`
- `cron.go`
- `validate.go`

#### Integracoes especificas

- `mcp_tool.go`
- `tts_send.go`
- `i2c.go`, `i2c_linux.go`, `i2c_other.go`
- `spi.go`, `spi_linux.go`, `spi_other.go`

### Contrato observavel das tools

As tools representam a maior superficie de efeito colateral do sistema. Para clean room, isso significa:

- o agente precisa poder agir sobre sistema, rede, arquivos e extensoes
- as tools sao descobertas, habilitadas e desabilitadas por configuracao
- o schema de parametros e parte do contrato de invocacao

### Riscos de seguranca

- shell abuse
- path traversal em operacoes de arquivo
- SSRF e fetch de hosts internos
- instalacao insegura de skills externas
- vazamento de segredos em parametros, logs ou outputs de tool
- acesso a hardware em ambiente inadequado

## 4. MCP, audio e media

### `pkg/mcp/`

- `manager.go`: lifecycle de servidores MCP
- `isolated_command_transport.go`: transporte por subprocesso isolado
- `manager_test.go`: cobertura

Contrato observavel:

- o sistema consegue anexar tools vindas de servidores MCP externos
- o lifecycle desses servidores nao e improvisado dentro do loop principal

### `pkg/audio/`

#### `audio/asr/`

- `asr.go`: factory
- `agent.go`: integracao com runtime
- `audio_model_transcriber.go`
- `elevenlabs_transcriber.go`
- `whisper_transcriber.go`

#### `audio/tts/`

- `tts.go`: factory
- `openai_tts.go`
- `mimo_tts.go`

Arquivos auxiliares:

- `ogg.go`
- `sentence.go`

Contrato observavel:

- suporte a audio inbound e outbound faz parte do produto
- a plataforma pode transcrever audio recebido e sintetizar resposta falada em alguns canais

### `pkg/media/`

- `store.go`: storage e lifecycle de midia
- `tempdir.go`: resolucao de diretorio temporario

## 5. Auth, credenciais, identidade e isolamento

### `pkg/auth/`

- `oauth.go`: flows OAuth
- `pkce.go`: PKCE
- `store.go`: persistencia auth
- `token.go`: parsing de token
- `anthropic_usage.go`: uso/billing especifico

### `pkg/credential/`

- `credential.go`: resolucao de credencial, incluindo esquemas como `file://` e `enc://`
- `keygen.go`: geracao de chave
- `store.go`: persistencia

### `pkg/identity/`

- `identity.go`: canonicalizacao e matching de identidades

### `pkg/isolation/`

- `runtime.go`: configuracao comum
- `platform_linux.go`: isolamento via bubblewrap ou equivalente
- `platform_windows.go`: comportamento em Windows
- `platform_other.go`: fallback para outras plataformas
- `README.md`: documentacao propria

### `pkg/devices/`

- `service.go`
- `source.go`
- `sources/usb_linux.go`, `usb_stub.go`
- `events/events.go`

## 6. Hotspots de QA em integracoes externas

Prioridade alta:

1. `pkg/tools/*`
2. `pkg/channels/manager.go` e canais com protocolo proprio (`weixin`, `wecom`, `pico`)
3. `pkg/providers/factory.go`, `fallback.go`, `error_classifier.go`
4. `pkg/mcp/manager.go`
5. `pkg/isolation/platform_linux.go`

Motivos:

- alta superficie de IO e efeitos colaterais
- compatibilidade externa fraca a mudanças internas
- grande chance de regressao silenciosa em runtime real

## 7. O que uma reimplementacao clean room deve preservar

- interface publica dos canais suportados como comportamento, nao como layout de codigo
- semantica de configuracao de providers e tools
- suporte a fallback, cooldown e roteamento basico de modelo
- capacidade de expor tools locais e MCP tools
- auth flows e resolucao de credenciais seguras
- restricoes operacionais de canais, tamanhos e formatos

## 8. O que deve ter identidade propria na reimplementacao

- organizacao dos adaptadores de canal
- tipo de runtime de tools
- desenho do sandbox
- forma de resolver auth e secrets internamente
- pipeline interno de parsing de tool calls
- desenho de abstractions para provider
