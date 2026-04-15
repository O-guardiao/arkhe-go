# 04 Build, Config, Docs e Workspace

## Build e Empacotamento

Os arquivos de operacao nao sao secundarios. Eles definem como o produto realmente chega ao usuario.

## Arquivos e Pastas-Chave

| Arquivo/Pasta | Papel |
| --- | --- |
| `Makefile` | build principal, deps e targets operacionais |
| `.goreleaser.yaml` | empacotamento multiplataforma |
| `.golangci.yaml` | politica de lint e qualidade |
| `docker/` | compose, Dockerfile, entrypoint e first-run em container |
| `scripts/` | instalacao/testes auxiliares |
| `examples/pico-echo-server/` | exemplo minimo de integracao |
| `config/config.example.json` | contrato publico de configuracao |
| `workspace/` | prompts/skills/memoria default do agente |

## Configuracao

O schema v2 faz parte do comportamento do produto. Os elementos mais importantes sao:

- `agents.defaults`;
- `model_list`;
- `channels.*`;
- `tools.*`;
- `gateway.*`;
- `voice`, `devices`, `heartbeat`.

Aspectos que a reimplementacao precisa preservar:

- defaults previsiveis;
- migracao de versao de config;
- segredos com serializacao segura na API;
- possibilidade de editar configuracao sem limpar segredos omitidos.

## `docs/`

Os documentos nao sao codigo, mas revelam requisitos implicitos. Areas com mais valor clean room:

- `configuration.md`
- `config-versioning.md`
- `providers.md`
- `credential_encryption.md`
- `security_configuration.md`
- `sensitive_data_filtering.md`
- `tools_configuration.md`
- `channels/`
- `design/`
- `agent-refactor/`

As traducoes replicam o mesmo contrato em outras linguas; nao agregam nova logica, mas confirmam quais comportamentos sao considerados publicos.

## `workspace/`

`workspace/` mostra que o produto assume um ambiente de agente pre-semeado. Os arquivos com mais valor sao:

- `AGENT.md`
- `USER.md`
- `SOUL.md`
- `skills/`
- `memory/`

Uma nova implementacao pode trocar o formato, mas nao deve perder a ideia de:

- persona/contrato do agente;
- skills default;
- memoria local e prompts embarcados.

## Riscos de Copia Disfarcada

- reproduzir `config.example.json` quase campo por campo sem repensar seu proprio schema;
- clonar a mesma nomenclatura publica (`allow_from`, `model_list`, `reasoning_channel_id`) quando um contrato proprio seria suficiente;
- recriar o mesmo conjunto de docs com a mesma seccao, ordem e naming;
- copiar a mesma composicao de workspace (`AGENT.md`, `SOUL.md`, `USER.md`) sem uma semantica propria.

## Sugestoes de Teste Essenciais

1. Arquivo de config legado deve migrar para o schema atual sem perder campos suportados.
2. Build local e build container devem gerar runtime funcional equivalente.
3. Config com segredo `enc://` deve carregar com passphrase valida e falhar de modo claro com passphrase ausente.
4. Workspace default ausente deve ser recriado ou degradar com erro acionavel.
5. Exemplo minimo em `examples/` deve continuar servindo como smoke test de integracao.
