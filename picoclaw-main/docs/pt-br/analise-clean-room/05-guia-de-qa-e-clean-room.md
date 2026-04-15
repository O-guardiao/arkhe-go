# Guia de QA e Clean Room Review

## Objetivo

Este documento traduz a leitura do repositorio em um metodo de revisao aplicavel a sua reimplementacao. Ele existe para impedir duas falhas comuns:

- cair em “paridade falsa”, onde o codigo parece similar mas quebra contratos importantes
- cair em “copia disfarçada”, onde a estrutura interna replica o original sem necessidade

## 1. Contratos de comportamento a preservar

### Testes de runtime core

- existe um processo principal de gateway/daemon
- o agente processa mensagens em loop com tool use iterativo
- steering e subturn permitem redirecionamento e execucao filha
- contexto, historico e sumario sao gerenciados com budget

### Configuracao

- existe `config.json` como centro de operacao
- providers, canais e tools sao configuraveis por arquivo
- variaveis de ambiente sobrescrevem partes do comportamento

### Integracoes

- multiplos canais podem publicar e receber mensagens
- multiplos providers LLM podem ser selecionados, roteados e colocados em fallback
- tools podem ser habilitadas/desabilitadas e executadas pelo agente
- MCP servers externos podem ampliar capacidades do sistema

### Superficies de usuario

- existe uma CLI operavel
- existe um launcher web separado do gateway
- existe um launcher TUI para cenarios headless

### Operacao

- o sistema suporta build multiplataforma
- o sistema possui modos one-shot, gateway e launcher
- persistencia local de sessoes/workspace faz parte do uso esperado

## 2. Sinais de copia disfarçada

Marcas que devem disparar alerta numa implementacao sua:

- mesmos nomes de structs, enums, helpers e registries sem necessidade tecnica
- mesma decomposicao de pacotes e subpastas
- mesma ordem sequencial de startup e mesmas funcoes privadas espelhadas
- mesmas fronteiras artificiais entre session/memory/context se isso nao nascer dos seus requisitos
- comentarios e nomes que so fazem sentido historico dentro do repo original

Se a sua implementacao parece “o mesmo repo com outra sintaxe”, ela falhou no criterio clean room.

## 3. Onde ser idiomatico e diferente

- modele seu runtime em torno dos seus invariantes, nao das classes do original
- consolide subsistemas se sua arquitetura ficar mais limpa
- troque storage/queue/abstraction layer desde que preserve comportamento externo
- escolha UX propria para launcher e TUI
- renomeie entidades para refletir seu dominio, nao o dominio herdado do repo analisado

## 4. Matriz de risco por area

| Area | Risco principal | Severidade |
| --- | --- | --- |
| runtime do agente | regressao sistêmica de comportamento | alta |
| tools | efeito colateral inseguro ou nao deterministicamente controlado | alta |
| channels | mensagem errada, formato quebrado, erro de thread/reply | alta |
| providers | fallback errado, erro de auth, parse de tool call | alta |
| launcher web | drift entre UI e gateway, auth bypass | alta |
| persistencia | perda de historico, truncacao incorreta, vazamento entre sessoes | alta |
| config | quebra de compatibilidade ou defaults inseguros | media-alta |
| release/build | binario funcional em uma plataforma e quebrado em outra | media |

## 5. Casos extremos que voce deve sempre perguntar

### Runtime

- o que acontece quando uma tool trava e o turno nao recebe resposta?
- como o sistema reage a steering no meio de uma cadeia de tool calls?
- child turns podem vazar resultado para a sessao errada?

### Testes de config

- o sistema sobe sem modelo default?
- `model_name` aponta para algo inexistente?
- env vars contradizem o arquivo?

### Testes de channels

- mensagem longa excede limite do canal?
- reply/thread/guild/team foram resolvidos corretamente?
- um retry duplica envio?

### Testes de providers

- rate limit temporario dispara retry, cooldown ou fallback?
- provider compativel OpenAI devolve tool call em formato diferente?
- timeout parcial de streaming deixa estado corrompido?

### Tools

- path traversal foi bloqueado?
- stdout/stderr com segredo entra em log?
- uma tool assíncrona pode ficar orfa e ainda mutar estado?

## 6. Seguranca: perguntas que nao podem faltar

- ha sanitizacao de caminho e sandbox de escrita?
- comandos externos rodam com privilegios desnecessarios?
- segredos sao filtrados antes de irem para prompt ou log?
- autenticacao do launcher protege mesmo as rotas sensiveis?
- MCP server externo pode ampliar privilegios do agente sem gate explicito?

## 7. Plano base de testes por subsistema

### Runtime core

- turno simples sem tool calls
- turno com multiplas tool calls e resposta final
- steering injetado no meio da execucao
- subturn com timeout e orfandade controlada
- budget overflow com compactacao/sumario

### Config

- carga de config valida minima
- config invalida com erro explicito
- migration de versao antiga
- env override prevalecendo sobre arquivo
- filtro de dados sensiveis preservando texto inocente

### Channels

- inbound -> bus -> outbound happy path
- mensagem longa com split correto
- markdown/HTML channel-specific
- media upload / attachment flow
- allow-list e auth do canal

### Providers

- chamada bem-sucedida
- rate limit com fallback/cooldown
- erro de credencial sem retry indevido
- parse de tool call
- roteamento light/heavy

### Testes do launcher web

- login com token
- session cookie valida
- start/stop/restart do gateway
- proxy Pico autenticado
- config update preservando segredos

## 8. Formato padrao de revisao que vou usar quando voce enviar codigo

Para qualquer modulo que voce enviar, a revisao deve seguir exatamente este formato:

### Status da Funcionalidade

- Aprovado
- Precisa de Ajustes

### Analise de Paridade e Originalidade

- a implementacao atinge ou nao o comportamento esperado
- divergencias de logica de negocio
- sinais de imitacao estrutural desnecessaria
- sugestoes de design mais idiomatico

### Edge Cases e Pontos Cegos

- entradas invalidas
- concorrencia
- timeout
- ordenacao de eventos
- gargalos de performance

### Seguranca e Performance

- superficies de ataque
- vazamento de segredo
- isolamento
- custo computacional, IO e memoria

### Casos de Teste Sugeridos

- 3 a 5 testes essenciais

## 9. Regra operacional para clean room

Use o repositorio original para entender:

- comportamento
- requisitos implicitos
- contratos de configuracao
- fluxos operacionais

Nao use o repositorio original para copiar:

- shape de pacotes
- nomes internos
- ordem de helpers privados
- organizacao historica da codebase

## 10. Prioridade de revisao quando o seu codigo chegar

Se voce mandar um modulo, a ordem mais eficaz de analise e:

1. comportamento observavel
2. riscos de seguranca
3. edge cases e concorrencia
4. identidade propria do design
5. estrategia de testes
