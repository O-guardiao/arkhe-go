# 03 Launcher Web e Operacao

## Tese Estrutural

O launcher web nao e um painel cosmetico. Ele e a camada operacional que:

- autentica acesso local;
- edita configuracao;
- inicia e para o gateway;
- faz proxy do canal Pico via WebSocket;
- mostra sessoes e logs;
- conduz onboarding e setup.

## Partes Principais

### `web/backend/`

Subareas mais importantes:

- `api/`: rotas de negocio (`config`, `models`, `gateway`, `pico`, `session`, `auth`, `startup`, `skills`, `tools`, `wecom`)
- `middleware/`: auth do dashboard, policy de IP, logger, referrer policy
- `dashboardauth/`: store bcrypt/SQLite para senha do painel
- `launcherconfig/`: porta, public exposure, CIDRs e token do launcher
- `utils/`: browser open, onboarding, runtime helpers

### `web/frontend/`

SPA React/TanStack que cobre:

- chat Pico;
- configuracao de modelos e canais;
- gestao de credenciais;
- visualizacao de logs e sessoes;
- login/setup do launcher.

## Fluxos que uma reimplementacao precisa preservar

### 1. Bootstrap local

- resolver `config.json` e `launcher-config.json`;
- subir backend HTTP local;
- opcionalmente abrir navegador;
- tentar auto-start do gateway;
- autenticar usuario local com token/senha.

### 2. Controle do gateway

- status running/stopped/starting;
- start com polling de health/pid file;
- stop/restart com attach a processo existente quando cabivel;
- buffer de logs recente para UI.

### 3. Chat Pico sobre a mesma porta do launcher

- frontend abre WS no launcher, nao direto no gateway;
- backend valida token, recompõe auth interna e faz reverse proxy;
- sessoes persistidas pelo gateway ficam browseaveis pela UI.

### 4. Edicao de configuracao sem perder segredo

- listar modelos com mascara e nao com segredo bruto;
- PATCH/PUT de config mantendo segredos omitidos pelo frontend;
- setup de canais/OAuth/WeCom salvando binding e reiniciando gateway quando preciso.

## O que nao deve ser copiado literalmente

- a mesma arvore `web/backend/api/*.go` + `web/frontend/src/features/*`;
- a mesma estetica visual do dashboard;
- o mesmo fluxo de query token no primeiro load;
- a mesma separacao entre `launcher-config` e `config` se outra estrutura operacional fizer mais sentido.

Preserve apenas o que o usuario percebe:

- existe um painel local;
- ele sobe o runtime;
- ele protege o acesso;
- ele permite configurar canais/modelos sem editar JSON cru;
- ele serve como ponto unico para UI e WebSocket.

## Edge Cases Operacionais

- PID file stale e processo ja morto;
- browser UI ligada, mas gateway reiniciando no meio do chat;
- segredo omitido na UI e patch parcial;
- login por senha inicial vs token de bootstrap;
- modo publico com allowlist CIDR invalida;
- WeCom/QR flow confirmando credencial incompleta;
- historico de sessao com JSONL grande ou linha corrompida.

## Sugestoes de Teste Essenciais

1. Start gateway com PID stale deve limpar o estado e subir processo novo.
2. Proxy Pico deve rejeitar token invalido e reconectar ao host/porta corretos apos restart.
3. PUT/PATCH de config deve preservar segredo omitido e atualizar apenas campos enviados.
4. Login inicial por token/senha deve resultar em cookie HttpOnly e acesso subsequente sem query token.
5. Listagem de sessoes deve tolerar JSONL com linhas invalidas sem derrubar toda a UI.
