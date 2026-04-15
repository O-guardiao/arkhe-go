# MCTS no PicoClaw

## Conclusão operacional

- MCTS canônico exige estado explícito, política de seleção, expansão, avaliação e backup sobre nós persistentes.
- O PicoClaw atual não busca em estados de ambiente fechados; ele busca em SubTurns de LLM.
- Portanto, a melhoria segura não é fingir AlphaZero em texto. É aproximar as partes úteis: diversidade controlada, profundidade real, retenção do melhor resultado e refinamento iterativo dos ramos fortes.

## O que a literatura e os projetos maduros realmente fazem

- UCT formaliza a seleção como exploração versus exploração com bônus estatístico. Referência canônica: Kocsis e Szepesvári, Bandit Based Monte-Carlo Planning (2006). Resumo acessível em https://www.chessprogramming.org/UCT.
- A survey moderna destaca que MCTS forte em produção quase sempre depende de hibridização, vieses estruturais e adaptações ao domínio, não de rollouts cegos puros. Referência: https://arxiv.org/abs/2103.04931.
- OpenSpiel documenta e implementa o básico de produção: UCT/PUCT, ação final escolhida pelo filho mais visitado, backup de estados resolvidos, suporte a chance nodes, ruído Dirichlet na raiz e políticas distintas para seleção e decisão final. Referências: https://openspiel.readthedocs.io/en/latest/algorithms.html e o código em https://github.com/google-deepmind/open_spiel/tree/main/open_spiel/algorithms/mcts.h.
- AlphaZero e MuZero não usam MCTS puro; usam busca guiada por prior e value. Sem prior/value, a qualidade cai bastante. Referências: https://arxiv.org/abs/1712.01815 e https://arxiv.org/abs/1911.08265.
- KataGo e Leela Chess Zero mostram o lado engenharia: cpuct dinâmico, FPU, root noise, temperatura apenas na escolha final da raiz, contadores in-flight e seleção final por visitas, não por um único rollout “bonito”. Referências úteis:
  - KataGo: https://github.com/lightvector/KataGo/blob/master/docs/GraphSearch.md
  - KataGo search params: https://github.com/lightvector/KataGo/tree/main/cpp/search/searchparams.h
  - Lc0 search params: https://github.com/LeelaChessZero/lc0/tree/main/src/search/classic/params.cc
  - Lc0 best-child selection: https://github.com/LeelaChessZero/lc0/tree/main/src/search/classic/search.cc
- Ramanujan, Sabharwal e Selman mostram um limite importante: UCT tende a perder traps rasos em domínios muito adversariais, como xadrez. Mais amostras não consertam sozinho um espaço de busca ruim. Referência: https://www.chessprogramming.org/UCT.

## O que isso implica para este repositório

- O MCTS antigo de pkg/recursion/mcts.go era só best-of-N paralelo: mesmo prompt, mesma profundidade lógica zero, Depth ignorado.
- Sem um estado explícito com ações legais, transições e reward confiável, ainda não existe base para um MCTS clássico com árvore persistente, visitas por nó, virtual loss e backup estatístico real.
- O passo de maior retorno e menor risco era transformar o mecanismo em busca iterativa sobre candidatos, preservando a infraestrutura já madura de SubTurn.

## Melhoria implementada

- Depth agora controla rounds reais de busca.
- O round inicial usa estratégias distintas de exploração por ramo, em vez de repetir o mesmo prompt em paralelo.
- Rounds posteriores refinam apenas os melhores sobreviventes do round anterior, em esquema de beam search inspirado em MCTS.
- O melhor resultado final é escolhido sobre todos os candidatos explorados, não apenas sobre o último round.
- O tool mcts_explore agora aceita depth opcional por chamada, além de branches.
- Testes novos cobrem profundidade real, existência de prompts de refinamento e diversidade inicial de estratégias.

## O que ainda não foi implementado

- UCT ou PUCT reais sobre uma árvore persistente de estados.
- Backup de valor por nó com contagem de visitas.
- Virtual loss ou coordenação fina de colisão entre workers.
- Transposições, graph search ou solved-state propagation.
- Prior/value learned evaluator para guiar seleção.

## Leitura honesta do estado atual

- Depois deste patch, o nome MCTS continua sendo um rótulo imperfeito.
- Tecnicamente, o mecanismo ficou mais próximo de uma busca iterativa e diversificada sobre candidatos de LLM do que de um MCTS clássico de jogos.
- Isso é aceitável no curto prazo porque melhora a utilidade ponta a ponta sem exigir uma reescrita do loop do agente nem inventar uma árvore que o runtime atual ainda não sabe sustentar.