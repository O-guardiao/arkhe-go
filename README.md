# arkhe-go

Repositorio de trabalho para manter, evoluir e versionar a fusao entre PicoClaw e a recriacao local de rlm-go.

## Estrutura

- `picoclaw-main/`: copia de trabalho do PicoClaw usada para integrar o runtime RLM.
- `rlm-go/`: recriacao local de rlm-go mantida neste repositorio como codigo canonico do projeto.

## Regra de trabalho

O diretorio `rlm-go/` deste repositorio nao veio de um repositorio original publico do GitHub.
Ele e a recriacao local existente nesta maquina e, a partir daqui, deve ser tratado como a base oficial de desenvolvimento dentro de `arkhe-go`.

O modulo Go em `picoclaw-main/go.mod` aponta para `../rlm-go`, tornando este repositorio autocontido para desenvolvimento e testes locais.
Os artefatos Docker em `picoclaw-main/docker/` devem buildar a partir da raiz de `arkhe-go` para carregar essa dependencia local; usar apenas `picoclaw-main/` como contexto perde a fusao com `rlm-go`.
