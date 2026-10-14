# VPN MULTISERVEUR

Plateforme VPN basée sur [WireGuard](https://www.wireguard.com/) : un **control plane** qui gère les utilisateurs, les serveurs VPN et les appareils, un **agent** installé sur chaque serveur VPN, et une **application mobile** qui monte le tunnel.

```
 App mobile ──HTTPS /api/v1──▶ API (control plane) ──gRPC──▶ agent (VPS FR) ── wg0
 (Expo / RN)                    Gin + PostgreSQL     ──gRPC──▶ agent (VPS US) ── wg0
      │                                              ──gRPC──▶ agent (VPS …)  ── wg0
      └──────────────── tunnel WireGuard (UDP 51820) ───────────────▶ serveur choisi
```

## Fonctionnalités

- Inscription / connexion par JWT, gestion du profil et du mot de passe.
- Gestion de plusieurs serveurs VPN (régions), chacun avec son propre subnet.
- Création d'appareils : génération des clés, allocation d'IP (IPv4 + IPv6 dérivée), rendu de la config `wg-quick`.
- Synchronisation des peers vers les serveurs par un agent gRPC, avec réconciliation périodique et remontée des derniers handshakes.
- Suivi des sessions VPN par appareil.
- Application iOS / Android qui récupère la config et pilote le tunnel natif.

## Structure du dépôt

| Dossier | Contenu |
|---------|---------|
| [`backend/`](backend/) | Module Go : API HTTP (`cmd/api`) et agent gRPC (`cmd/agent`), migrations SQL, scripts de provisioning |
| `mobile/` | App Expo SDK 54 / React Native (expo-router, TypeScript) — **ignorée par ce dépôt** (`.gitignore`), versionnée séparément |

## Stack technique

- **Backend** : Go 1.25, Gin, GORM, PostgreSQL 16, gRPC / Protobuf, `wgctrl`, golang-migrate.
- **Mobile** : Expo SDK 54, React Native 0.81, expo-router, `react-native-wireguard-vpn` (Network Extension sur iOS, VpnService sur Android).
- **Déploiement** : Docker / Docker Compose, un VPS pour le control plane et un VPS par région.

## Démarrage rapide (backend)

Prérequis : Go 1.25+, Docker, [golang-migrate](https://github.com/golang-migrate/migrate) (CLI `migrate`).

```bash
cd backend
docker compose -f docker-compose.dev-db.yml up -d   # PostgreSQL 16 local
cp .env.example .env                                # puis ajuster les variables
make setup-db                                       # migrations + données de seed
make run                                            # API sur http://localhost:8080
```

Dans un autre terminal, un agent en mode mémoire (ni root ni interface `wg0` nécessaires) :

```bash
cd backend
make run-agent
```

Vérification : `curl http://localhost:8080/health` → `{"status":"ok"}`.

### Commandes utiles

| Commande | Effet |
|----------|-------|
| `make dev` | API avec rechargement à chaud (air) |
| `make test` | `go test ./... -cover` |
| `make fmt` | `go fmt ./...` |
| `make build` / `make build-agent` | binaires de l'API / de l'agent (linux/amd64 par défaut) |
| `make proto` | régénère `gen/` depuis `api/proto/agent/v1/agent.proto` |
| `make migration name=xxx` | nouvelle paire de migrations SQL |

## Application mobile

```bash
cd mobile
npm install      # applique aussi les patches (patch-package)
npm run ios      # ou : npm run android
```

Le tunnel s'appuie sur du code natif : **Expo Go ne fonctionne pas**, il faut un development build. Sur iOS, la Network Extension demande des étapes manuelles dans Xcode (voir `mobile/docs/VPN_NATIVE_SETUP.md`).

En développement, l'URL de l'API est déduite de l'hôte Metro ; en production elle est fournie par `EXPO_PUBLIC_API_URL`.

## API

Toutes les routes sont préfixées par `/api/v1`. Seules `/auth/register` et `/auth/login` sont publiques ; les autres attendent un en-tête `Authorization: Bearer <JWT>`.

| Ressource | Routes principales |
|-----------|--------------------|
| Auth | `POST /auth/register`, `POST /auth/login` |
| Utilisateur | `GET/PUT/DELETE /user/me`, `PUT /user/me/password` |
| Serveurs VPN | `POST /server`, `GET /server/list`, `GET/PUT/DELETE /server/:id` |
| Appareils | `POST /device`, `GET /device/list`, `GET/PUT/DELETE /device/:id`, `GET /device/:id/config`, `GET/POST /device/:id/session` |
| Agent | `GET /server/:id/agent/status`, `GET /server/:id/agent/peers`, `POST /server/:id/agent/sync` |

### Cycle de vie d'un appareil

1. L'app crée un appareil : l'API choisit un serveur, alloue une IP dans son subnet et enregistre le peer.
2. L'API pousse le peer à l'agent du serveur ; la base reste la source de vérité et le réconciliateur corrige tout écart.
3. L'app récupère sa config via `GET /device/:id/config`, monte le tunnel, puis déclare la session.

## Configuration

Les variables sont documentées dans [`backend/.env.example`](backend/.env.example). Les plus importantes :

| Variable | Rôle |
|----------|------|
| `DB_*` | connexion PostgreSQL |
| `JWT_SECRET`, `JWT_EXPIRATION_HOURS` | signature et durée des tokens |
| `WG_DEFAULT_SUBNET`, `WG_DNS`, `WG_ALLOWED_IPS`, `WG_MTU` | paramètres par défaut des configs clientes |
| `WG_IPV6_PREFIX` | préfixe IPv6 du tunnel, identique sur l'API et les serveurs |
| `AGENT_TOKEN` | secret partagé entre l'API et les agents |
| `AGENT_TLS_CA` | CA des certificats des agents (vide = clair, dev uniquement) |
| `AGENT_SYNC_ENABLED`, `AGENT_SYNC_INTERVAL` | réconciliation périodique des peers |

> En production, changez impérativement `JWT_SECRET` et `AGENT_TOKEN`, et activez TLS entre l'API et les agents.

## Déploiement

- [`backend/docs/DEPLOYMENT.md`](backend/docs/DEPLOYMENT.md) — plateforme complète en local (Docker Compose, 3 régions simulées) puis en production sur plusieurs VPS.
- [`backend/docs/AJOUTER_SERVEUR.md`](backend/docs/AJOUTER_SERVEUR.md) — ajouter un serveur VPN (`scripts/setup-wg-server.sh`).
- [`backend/docs/AGENT_GRPC.md`](backend/docs/AGENT_GRPC.md) — protocole entre l'API et les agents.
- Modèles Compose : [`example.docker-compose.prod-api.yml`](backend/example.docker-compose.prod-api.yml) et [`example.docker-compose.prod-agent.yml`](backend/example.docker-compose.prod-agent.yml).

## Contribuer

- Le schéma de base est géré uniquement par les migrations SQL : toute modification d'un modèle demande une nouvelle migration.
- Le code gRPC de `backend/gen/` est généré : modifier le `.proto`, puis lancer `make proto`.
- Lancer `make fmt` et `make test` avant de proposer une merge request.

Les conventions détaillées du backend sont dans [`backend/CLAUDE.md`](backend/CLAUDE.md).
