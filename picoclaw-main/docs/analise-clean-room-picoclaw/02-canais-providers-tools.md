# 02 Canais, Providers e Tools

## Leitura Estrutural

Esta e a massa mais delicada para clean room porque mistura comportamento visivel ao usuario, integracoes externas e muita tentacao de copiar estrutura por semelhanca.

## `pkg/channels/`

### Papel das integracoes de modelo

`pkg/channels/` nao e so um conjunto de adapters. Ele define:

- autorizacao minima por remetente (`allow_from`);
- heuristica de grupo/mention/prefix;
- streaming, placeholder, typing e reaction UX;
- splitting de mensagens longas;
- rate limiting por canal;
- multiplexacao entre transporte e `MessageBus`.

### Contratos de provider que devem sobreviver

- cada canal exposto deve conseguir `Start`, `Stop`, `Send`, `IsRunning`;
- allowlist vazia hoje implica comportamento permissivo;
- suporte a reasoning channel separado e placeholders e opcional, mas parte do UX observado;
- manager precisa absorver diferencas de rate limit e tamanho maximo por plataforma.

### Risco clean room

Nao copie o mesmo padrao de `BaseChannel` + `registry init()` + `Manager` com as mesmas capacidades opcionais. O contrato que importa e:

- transporte uniforme;
- politicas por canal;
- capabilities declarativas;
- adaptacao de UX por plataforma.

## `pkg/providers/`

### Papel das ferramentas em runtime

Os providers fazem mais do que chamar APIs:

- normalizam modelos e credenciais;
- sustentam fallback cross-provider;
- aplicam cooldown e rate limit por identidade de modelo;
- diferenciam suporte a streaming, thinking e busca nativa.

### Comportamento que precisa sobreviver

- configuracao baseada em `model_list`;
- alias/modelo default por agente;
- fallback com classificacao de erro e cooldown;
- round-robin ou outra politica quando ha alias duplicado.

### Identidade propria recomendada

Em vez de repetir `FallbackChain`, voce pode modelar:

- `model resolver`;
- `candidate policy`;
- `failure classifier`;
- `provider runtime`.

## `pkg/tools/`

### Papel real

As tools sao o braco executor do agente. As mais importantes para clean room sao:

- `exec`;
- `cron`;
- `web`/`web_fetch`;
- `filesystem`/`edit_file`/`append_file`;
- `message`/`reaction`/`send_file`;
- `load_image`;
- `mcp_tool`;
- `subagent`.

### Contratos observaveis

- schema de parametros exposto ao modelo;
- contexto de canal/chat/message injeta escopo na tool;
- respostas de tool podem ser sincronas, assincronas ou silenciosas;
- `exec` e `cron` carregam maior risco operacional;
- MCP e skills ampliam dinamicamente a superficie do agente.

## `pkg/skills/` e `pkg/mcp/`

### O que importa por comportamento

- descoberta de skills por metadados;
- elegibilidade condicionada a binarios e ambiente;
- ativacao de servidores MCP em runtime;
- ranking/relevancia e injecao controlada em contexto.

Nao copie o formato literal de `SKILL.md` ou o mesmo ranking. Preserve a ideia de:

- catalogo declarativo;
- filtro de elegibilidade;
- ativacao sob demanda;
- custo/risco considerado na exposicao ao modelo.

## `pkg/credential/` e seguranca de segredo

O pacote de credencial resolve:

- plaintext;
- `file://` relativo ao diretorio de config;
- `enc://` com AES-256-GCM + HKDF-SHA256;
- passphrase em memoria/processo.

Para clean room, a regra correta e preservar o contrato de resolucao, nao o mesmo layout de funcoes.

## Edge Cases Criticos

- `allow_from` vazio + canal habilitado;
- attachments grandes quebrando streaming ou splitting;
- provider em cooldown sem candidato alternativo;
- tool assicrona finalizando depois do timeout de conversa;
- MCP server travado ou vazando processo;
- segredos `file://` apontando para symlink fora do diretorio permitido.

## Sugestoes de Teste Essenciais

1. Canal em grupo com mention vs prefix vs sem trigger deve responder apenas quando a politica permitir.
2. Mensagem longa com bloco de codigo deve ser dividida sem quebrar semantica visivel.
3. Falha de provider classificada como retriavel deve cair para candidato seguinte com cooldown do anterior.
4. Credencial `file://` por symlink fora do config dir deve ser rejeitada.
5. Tool `exec` ou equivalente deve respeitar isolamento, timeout e contexto de canal.
6. Skill elegivel sem binario ausente deve ser ativada; sem binario deve ser descartada com erro explicito.
7. MCP server travado deve expirar por timeout e nao deixar processo zumbi.
8. Streaming de canal com placeholder deve finalizar uma vez so e sem duplicar entrega.
9. Round-robin entre duas entradas com mesmo alias deve distribuir chamadas ou seguir a politica declarada.
10. Tool `message` deve publicar uma unica resposta mesmo quando outra tool ja entregou conteudo.
