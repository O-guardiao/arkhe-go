# 99 Lista de Arquivos Prioritarios

Esta lista nao tenta narrar os 1076 arquivos individualmente. Ela isola os arquivos com maior densidade comportamental para clean room. Os arquivos nao listados aqui sao, em sua maioria, testes de cobertura, docs traduzidas, assets estaticos, wrappers finos ou variantes que seguem o mesmo contrato estrutural.

## Root e Operacao

- `README.md`
- `README.pt-br.md`
- `ROADMAP.md`
- `Makefile`
- `go.mod`
- `.golangci.yaml`
- `.goreleaser.yaml`
- `config/config.example.json`

## Entrypoints

- `cmd/picoclaw/main.go`
- `cmd/picoclaw/internal/agent/*`
- `cmd/picoclaw/internal/gateway/*`
- `cmd/picoclaw/internal/onboard/*`
- `cmd/picoclaw/internal/auth/*`
- `cmd/picoclaw/internal/status/*`
- `cmd/picoclaw-launcher-tui/main.go`
- `cmd/picoclaw-launcher-tui/ui/gateway.go`
- `cmd/picoclaw-launcher-tui/ui/home.go`

## Runtime Core

- `pkg/gateway/gateway.go`
- `pkg/agent/loop.go`
- `pkg/agent/turn.go`
- `pkg/agent/subturn.go`
- `pkg/agent/events.go`
- `pkg/agent/eventbus.go`
- `pkg/agent/context_manager.go`
- `pkg/agent/instance.go`
- `pkg/agent/registry.go`
- `pkg/agent/steering.go`
- `pkg/bus/bus.go`
- `pkg/bus/types.go`
- `pkg/session/manager.go`
- `pkg/session/jsonl_backend.go`
- `pkg/state/state.go`
- `pkg/health/server.go`

## Config, Migracao e Segredos

- `pkg/config/config.go`
- `pkg/config/config_struct.go`
- `pkg/config/defaults.go`
- `pkg/config/gateway.go`
- `pkg/migrate/*`
- `pkg/credential/credential.go`
- `pkg/credential/store.go`
- `pkg/credential/keygen.go`

## Canais

- `pkg/channels/base.go`
- `pkg/channels/manager.go`
- `pkg/channels/registry.go`
- `pkg/channels/dynamic_mux.go`
- `pkg/channels/interfaces.go`
- `pkg/channels/split.go`
- `pkg/channels/pico/*`
- `pkg/channels/telegram/*`
- `pkg/channels/discord/*`
- `pkg/channels/slack/*`
- `pkg/channels/wecom/*`
- `pkg/channels/weixin/*`
- `pkg/channels/irc/*`
- `pkg/channels/matrix/*`
- `pkg/channels/feishu/*`
- `pkg/channels/whatsapp/*`
- `pkg/channels/whatsapp_native/*`

## Providers e Routing

- `pkg/providers/types.go`
- `pkg/providers/factory.go`
- `pkg/providers/fallback.go`
- `pkg/providers/error_classifier.go`
- `pkg/providers/cooldown.go`
- `pkg/providers/ratelimiter.go`
- `pkg/providers/model_ref.go`
- `pkg/providers/openai_provider.go`
- `pkg/providers/anthropic_provider.go`
- `pkg/providers/azure_provider.go`
- `pkg/providers/bedrock_provider.go`
- `pkg/providers/gemini_provider.go`
- `pkg/routing/router.go`
- `pkg/routing/classifier.go`

## Tools e Extensibilidade

- `pkg/tools/base.go`
- `pkg/tools/registry.go`
- `pkg/tools/shell.go`
- `pkg/tools/cron.go`
- `pkg/tools/web.go`
- `pkg/tools/filesystem.go`
- `pkg/tools/edit.go`
- `pkg/tools/message.go`
- `pkg/tools/send_file.go`
- `pkg/tools/load_image.go`
- `pkg/tools/mcp_tool.go`
- `pkg/tools/subagent.go`
- `pkg/skills/*`
- `pkg/mcp/manager.go`

## Audio, Media e Device Layer

- `pkg/audio/asr/*`
- `pkg/audio/tts/*`
- `pkg/media/store.go`
- `pkg/devices/*`
- `pkg/heartbeat/service.go`

## Launcher Web Backend

- `web/backend/main.go`
- `web/backend/api/router.go`
- `web/backend/api/config.go`
- `web/backend/api/models.go`
- `web/backend/api/gateway.go`
- `web/backend/api/pico.go`
- `web/backend/api/session.go`
- `web/backend/api/auth.go`
- `web/backend/api/startup.go`
- `web/backend/api/skills.go`
- `web/backend/api/tools.go`
- `web/backend/api/wecom.go`
- `web/backend/middleware/launcher_dashboard_auth.go`
- `web/backend/middleware/access_control.go`
- `web/backend/middleware/referrer_policy.go`
- `web/backend/dashboardauth/*`
- `web/backend/launcherconfig/config.go`

## Launcher Web Frontend

- `web/frontend/src/routes/*`
- `web/frontend/src/features/chat/*`
- `web/frontend/src/features/models/*`
- `web/frontend/src/features/credentials/*`
- `web/frontend/src/features/channels/*`
- `web/frontend/src/features/agent/*`
- `web/frontend/src/lib/*`
- `web/frontend/src/api/*`
- `web/frontend/src/i18n/*`

## Docs com maior valor comportamental

- `docs/configuration.md`
- `docs/config-versioning.md`
- `docs/providers.md`
- `docs/tools_configuration.md`
- `docs/credential_encryption.md`
- `docs/security_configuration.md`
- `docs/sensitive_data_filtering.md`
- `docs/cron.md`
- `docs/subturn.md`
- `docs/steering.md`
- `docs/spawn-tasks.md`
- `docs/chat-apps.md`
- `docs/docker.md`
- `docs/troubleshooting.md`
- `docs/agent-refactor/README.md`

## Workspace embarcado

- `workspace/AGENT.md`
- `workspace/USER.md`
- `workspace/SOUL.md`
- `workspace/skills/**`
- `workspace/memory/**`

## Regra pratica

Se voce for revisar somente 10% do repositorio para reconstruir o produto por comportamento, revise primeiro os arquivos desta lista. O restante serve como multiplicador de cobertura, nao como origem da arquitetura.
