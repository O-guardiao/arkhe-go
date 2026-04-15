# Launchers e Superficies de Usuario

## Escopo

Este documento cobre:

- `web/`
- `cmd/picoclaw-launcher-tui/`
- relacao entre launchers e o subprocesso `picoclaw gateway`

## 1. Launcher web: arquitetura real

O launcher web e um backend Go com frontend React incorporado. Ele nao e um simples frontend estatico.

### Papel do backend do launcher

- parse de flags (`-port`, `-public`, `-no-browser`, `-console`, `-debug`)
- onboarding automatico quando falta configuracao
- autenticacao do dashboard
- controle do subprocesso do gateway
- exposicao de APIs REST
- proxy WebSocket para o canal Pico
- integracao com systray em plataformas suportadas

### Arquivos principais de `web/backend/`

- `main.go`: bootstrap do servidor HTTP
- `app_runtime.go`: lifecycle e shutdown
- `embed.go`: embedding do frontend compilado
- `i18n.go`: linguagem
- `systray.go` e variantes: tray integration

Familias importantes:

- `api/*`: handlers REST
- `middleware/*`: auth, access control, referrer policy, logging
- `launcherconfig/*`: configuracao do launcher (`launcher-config.json`)
- `dashboardauth/*`: password/token/session store
- `utils/*`: onboard, runtime helpers, browser open

## 2. Superficie HTTP do launcher

Pela estrutura de `web/backend/api/`, o launcher oferece pelo menos estes grupos de API:

- auth
- gateway
- config
- models
- model_status
- channels
- pico
- oauth
- weixin / wecom helpers
- tools
- skills
- session
- startup
- launcher_config
- log
- version
- update

Leitura correta:

- o launcher e um control plane local
- ele centraliza configuracao, login e inspeccao do runtime
- ele esconde complexidade do gateway por tras de uma camada HTTP coerente

## 3. Middleware e seguranca do launcher

Arquivos relevantes:

- `middleware/launcher_dashboard_auth.go`
- `middleware/access_control.go`
- `middleware/referrer_policy.go`
- `api/auth_login_limiter.go`

Controles observados:

- token do dashboard
- cookie de sessao assinado
- bearer auth para API
- allowlist CIDR opcional quando exposto publicamente
- referrer policy endurecida
- rate limiting no login

Riscos de QA e seguranca:

- bypass de auth em rotas novas
- inconsistencias entre login por token e sessao cookie
- exposure indevida em `-public`
- quebra do auto-login quando o launcher injeta `?token=`
- drift entre status real do gateway e o que a UI acredita

## 4. Frontend React do launcher

### Estrutura geral de `web/frontend/`

Infraestrutura:

- `package.json`
- `vite.config.ts`
- `tsconfig*.json`
- `eslint.config.js`
- `prettier.config.js`

Entradas e estado:

- `src/main.tsx`
- `src/index.css`
- `src/store/*`

Rotas:

- `src/routes/__root.tsx`
- `src/routes/index.tsx`
- `src/routes/models.tsx`
- `src/routes/credentials.tsx`
- `src/routes/config.tsx`
- `src/routes/config.raw.tsx`
- `src/routes/logs.tsx`
- `src/routes/launcher-login.tsx`
- `src/routes/launcher-setup.tsx`
- `src/routes/agent.tsx`
- `src/routes/agent/skills.tsx`
- `src/routes/agent/tools.tsx`
- `src/routes/agent/hub.tsx`
- `src/routes/channels/route.tsx`
- `src/routes/channels/$name.tsx`

Cliente de API:

- `src/api/http.ts`
- `src/api/gateway.ts`
- `src/api/pico.ts`
- `src/api/models.ts`
- `src/api/channels.ts`
- `src/api/oauth.ts`
- `src/api/skills.ts`
- `src/api/tools.ts`
- `src/api/sessions.ts`
- `src/api/system.ts`
- `src/api/launcher-auth.ts`

Hooks e features:

- `src/hooks/use-pico-chat.ts`
- `src/hooks/use-gateway.ts`
- `src/hooks/use-gateway-logs.ts`
- `src/hooks/use-session-history.ts`
- `src/features/chat/*`

### Leitura correta

O frontend nao e apenas uma pagina de chat. Ele e uma console de operacao local para:

- conversar com o agente
- gerir modelos e credenciais
- configurar canais
- editar configuracao do agente
- controlar gateway e logs
- navegar skills e tools

## 5. Launcher TUI

### Estrutura

Arquivos relevantes:

- `cmd/picoclaw-launcher-tui/main.go`
- `cmd/picoclaw-launcher-tui/config/config.go`
- `cmd/picoclaw-launcher-tui/ui/app.go`
- `ui/home.go`
- `ui/schemes.go`
- `ui/users.go`
- `ui/models.go`
- `ui/channels.go`
- `ui/gateway.go`

### Papel

O launcher TUI oferece uma superficie sem navegador para:

- setup local
- escolha de modelos
- ajuste de canais
- controle do gateway
- sincronizacao com a config principal

Ele existe para cenarios headless, SSH, VPS ou preferencia por terminal.

## 6. Contratos de clean room aqui

O que precisa ser preservado em comportamento:

- existencia de uma superficie grafica/web que simplifica o uso do gateway
- separacao entre launcher e gateway como processos distintos
- possibilidade de gerenciar modelos, credenciais, canais e logs sem editar JSON na mao
- uma superficie alternativa em terminal para setup e operacao

O que nao precisa ser igual:

- stack React exata
- organizacao dos componentes
- TanStack Router, Jotai ou shadcn como escolhas obrigatorias
- desenho do TUI

## 7. Gaps de QA observados

- o backend do launcher tem boa cobertura em Go, mas o frontend nao mostra suite equivalente de testes automatizados
- a combinacao auth + proxy + subprocesso e propensa a bugs de estado dificilmente capturados por unit tests puros
- a experiencia de erro sob onboarding incompleto ou gateway parcialmente funcional merece mais cenarios end-to-end

## 8. O que valeria documentar ainda mais

- mapa de rotas REST do launcher com metodos e auth
- mapa de rotas React e pagina/responsabilidade
- sequencia exata de start/attach/restart do gateway
- matriz de seguranca do dashboard em modo local versus publico
