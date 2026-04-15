# 06 Matriz de Testes

## Runtime Core

1. Conversa multi-turno preserva historico, summary e ordem de mensagens por sessao.
2. Slash command valido desvia do LLM; comando desconhecido cai para fluxo normal.
3. Fallback entre modelos funciona quando o primeiro provider falha por timeout/rate limit.
4. Shutdown durante turno em execucao nao corrompe sessao persistida.
5. Reload de config troca provider/canal sem matar processo principal.

## Sessao e Estado

1. Nome de arquivo de sessao sanitiza chaves compostas sem criar subdiretorios.
2. Escrita atomica sobrevive a crash entre tmp e rename.
3. Sessao vazia nao quebra listagem nem recuperacao.
4. Historico truncado preserva ultimas N mensagens corretas.
5. Arquivo JSONL com linha invalida nao derruba a leitura inteira.

## Canais

1. `allow_from` vazio, `*` explicito e allowlist especifica geram politicas distintas e verificaveis.
2. Grupo com `mention_only` ignora mensagem nao mencionada.
3. Prefixo em grupo remove o prefixo e preserva resto do conteudo.
4. Placeholder + streaming + envio final nao duplicam entrega.
5. Split de mensagens longas preserva blocos de codigo e markdown minimamente valido.

## Providers e Routing

1. Alias duplicado com round-robin ou politica equivalente distribui chamadas entre candidatos.
2. Erro nao retriavel aborta fallback imediatamente.
3. Cooldown impede reuse imediato de candidato recem-falho.
4. Light model so deve ser escolhido para prompts simples conforme politica declarada.
5. Credencial `enc://` falha com erro acionavel quando passphrase esta ausente.

## Tools e MCP

1. `exec` ou equivalente respeita timeout, cwd isolado e bloqueio a canais remotos.
2. `cron` agenda lembrete normal em qualquer canal, mas comando apenas em canal interno.
3. Tool de filesystem rejeita path traversal e path fora do workspace.
4. MCP server com binario ausente falha de modo explicito e nao fica meio-ativo.
5. Tool async publica resultado uma vez so e marca estado de conclusao corretamente.

## Launcher Web

1. `GET /api/models` mascara chaves; update parcial preserva segredo omitido.
2. Login bootstrap limpa a URL apos autenticar e produz cookie HttpOnly.
3. Gateway start/stop/restart atualiza status mesmo com PID stale.
4. Proxy Pico rejeita token invalido e troca alvo apos restart do gateway.
5. Sessao listada na UI corresponde ao transcript persistido no runtime.

## Seguranca Operacional

1. Exposicao publica sem CIDR permitido deve ser bloqueada ou explicitamente reconhecida.
2. `allow_from` vazio em canal remoto deve disparar aviso forte ou falhar fechado na sua reimplementacao.
3. `exec` remoto deve nascer desabilitado e exigir opt-in verificavel.
4. Endpoints administrativos nao devem expor segredos nem metadados em excesso sem auth.
5. Query token de bootstrap, se existir, nao deve sobreviver apos primeiro redirect nem aparecer em logs aplicacionais.

## Build e Deploy

1. Build local, build Docker e artefato empacotado iniciam o runtime com mesmo comportamento.
2. `config.example.json` ou equivalente gera primeira execucao consistente.
3. Container first-run cria config/workspace corretamente e degrada com erro acionavel se faltar segredo.
4. Upgrades de versao de config preservam campos ainda suportados.
5. Teste smoke do exemplo minimo cobre gateway + modelo fake + chat basico.
