# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Backend de la plateforme VPN WireGuard (module Go `backend`) : un control plane HTTP et un agent gRPC par serveur VPN. Commentaires de code en anglais, docs en français.

## Commandes

Toutes les commandes se lancent depuis `backend/` :

```bash
docker compose -f docker-compose.dev-db.yml up -d   # Postgres 16 local
cp .env.example .env
make migrate-up          # golang-migrate, fichiers SQL dans migrations/
make setup-db            # migrate-up + seed-up (seed/ a sa propre table de version seed_schema_migrations)
make run                 # API sur :8080 (make dev = hot reload via air)
make run-agent           # agent avec AGENT_BACKEND=memory (ni root ni wg0 requis) ;
                         # refuse de démarrer sans AGENT_TOKEN, sauf si AGENT_INSECURE_NO_AUTH=true
make test                # go test ./... -cover
go test ./internal/wireguard -run TestRenderClientConfig -v   # un seul test
make fmt                 # go fmt ./...
make build-agent         # binaire statique linux/amd64 (surcharger GOOS/GOARCH)
make proto               # régénère gen/ (protoc, protoc-gen-go, protoc-gen-go-grpc requis, cf. scripts/gen-proto.sh)
make migration name=xxx  # nouvelle paire de migrations séquentielles (seed-migration pour seed/)
```

Les tests existants couvrent `internal/wireguard`, `internal/agentproto` et `internal/orchestrator` ; ils ne demandent ni base ni agent.

## Règles

- Le schéma est géré uniquement par les migrations SQL (pas d'`AutoMigrate` GORM) : toute modification d'un modèle nécessite une nouvelle migration.
- Le code gRPC de `gen/` est commité. Ne jamais l'éditer à la main : modifier `api/proto/agent/v1/agent.proto` puis lancer `make proto`.
- Les erreurs métier sont des `*AppError` créées via `pkg/errors` (`NewNotFound`, `NewConflict`, `NewInvalidInput`, `NewUnavailable`…) et renvoyées par les handlers avec `apperrors.Handle(c, err)`. Toute autre erreur devient un 500 générique.

## Architecture

Deux binaires :
- **`cmd/api`** : le control plane, API HTTP Gin + PostgreSQL (GORM). Tout le câblage (repositories → services → handlers → routes) est dans `internal/app/app.go`.
- **`cmd/agent`** : tourne sur chaque serveur VPN ; serveur gRPC qui expose l'interface `wg0` locale (`internal/agent`). Backend `wgctrl` (kernel) ou `memory` (dev).

**L'API est le client gRPC, l'agent est le serveur** : l'API se connecte au `grpc_endpoint` de chaque serveur (auth par bearer `AGENT_TOKEN`, TLS optionnel via `AGENT_TLS_CA`). L'agent n'appelle jamais l'API.

Les packages métier de `internal/` (`user`, `auth`, `device`, `vpn-server`) suivent la même structure en couches : `model` → `repository` → `service` → `handler` → `router`, avec `dto.go`/`mapper.go`.

Routes : tout est sous `/api/v1` ; seuls `/auth/register` et `/auth/login` sont publics. `auth.Middleware` valide le JWT et place `userID` et `email` dans le contexte Gin. Il n'y a pas de notion de rôle : tout utilisateur authentifié peut gérer les serveurs et les autres utilisateurs. Le CORS n'est activé que hors production.

### Flux device → tunnel

1. `device.Service` choisit un serveur, alloue une IP via `wireguard.Pool` (IPAM sur le subnet **propre à chaque serveur**, `WG_DEFAULT_SUBNET` sert de valeur par défaut) et enregistre le device. Une IP n'est unique qu'au sein d'un serveur (index `(server_id, assigned_ip)`).
2. Il pousse le peer via l'interface `device.PeerPublisher`, implémentée par `orchestrator` (interface pour que `device` n'importe pas `orchestrator` ; `noopPublisher` sert quand aucun agent n'est configuré). Les échecs de push sont **journalisés, pas renvoyés** : la base est la source de vérité.
3. `orchestrator.Reconciler` (activé par `AGENT_SYNC_ENABLED`, lancé par `App.Start`) appelle périodiquement `SyncPeers` sur chaque serveur actif avec l'ensemble de peers attendu, met à jour `device.last_handshake` et `server.current_peers`. Tous les RPC de l'agent sont idempotents, donc rejouables sans risque.
4. Le client récupère sa config wg-quick via `GET /devices/:id/config`, puis déclare la connexion avec `POST /devices/:id/session` (table `vpn_session`, modèle `device.Session`).

Chaque device a aussi une IPv6 de tunnel **dérivée** de son IPv4 (`wireguard.DeviceIPv6`, préfixe `WG_IPV6_PREFIX`) : pas de colonne en base. Le préfixe doit être le même sur l'API et sur les serveurs (`setup-wg-server.sh`), qui font du NAT66 si le VPS a de l'IPv6, sinon rejettent l'IPv6 immédiatement. Les AllowedIPs sont convertis en CIDR par `hostIPWithCIDR` **dans l'agent** : un changement de format d'adresse impose de déployer les agents avant l'API.

`orchestrator/adapters.go` fait le pont entre `vpn-server`/`device` et les interfaces `ServerSource`/`PeerSource` de l'orchestrateur ; `orchestrator/pool.go` garde une connexion gRPC persistante par serveur ; `orchestrator/router.go` expose `/server/:id/agent/{status,peers,sync}` pour inspecter un agent ou forcer une réconciliation. `internal/agentproto` convertit proto ↔ types `wireguard`.

`internal/wireguard` est volontairement indépendant de la base et du HTTP (clés, IPAM, rendu des `.conf`, interface `Client` avec implémentations wgctrl et mémoire). `keys.go` ne dépend que de `x/crypto`, pas de `wgctrl`.

Pour aller plus loin : `internal/wireguard/README.md`, `docs/AGENT_GRPC.md` (protocole), `docs/AJOUTER_SERVEUR.md` (ajout d'un serveur VPN, `scripts/setup-wg-server.sh`), `docs/DEPLOYMENT.md`.
