# Déploiement : local et production

Ce guide explique comment faire tourner la plateforme complète — API, base de données et agents WireGuard — d'abord **en local avec Docker Compose** (3 régions simulées), puis **en production sur plusieurs VPS**. Pour comprendre *comment* l'API et les agents communiquent, voir [`AGENT_GRPC.md`](AGENT_GRPC.md).

```
Partie 1 — Local        docker compose : db + api + agent-fr / agent-us / agent-jp
Partie 2 — Production   VPS « control plane » (API + PostgreSQL) + un VPS par région (agent)
Partie 3 — Exploitation ajouter une région, rotation du token, diagnostic
```

---

# Partie 1 — Local (Docker Compose)

## 1.1 Ce que ça monte

```
                    ┌──────────────── docker network ────────────────┐
localhost:8080 ───▶ │ api ──gRPC──▶ agent-fr:50051  wg0 10.8.0.1/24  │ ◀── udp 51820
                    │     ──gRPC──▶ agent-us:50051  wg0 10.9.0.1/24  │ ◀── udp 51821
                    │     ──gRPC──▶ agent-jp:50051  wg0 10.10.0.1/24 │ ◀── udp 51822
localhost:5432 ───▶ │ db (PostgreSQL)                                │
                    └────────────────────────────────────────────────┘
```

| Service | Rôle | Se termine ? |
|---------|------|--------------|
| `db` | PostgreSQL 16, volume `pg_data` | non |
| `migrate` | applique `migrations/` | oui, après succès |
| `seed-servers` | insère les 3 serveurs de `seed/` | oui, après succès |
| `api` | control plane, `http://localhost:8080` | non |
| `agent-fr`, `agent-us`, `agent-jp` | un agent par région, chacun avec son `wg0` | non |

Les agents créent une **vraie interface WireGuard** dans leur conteneur. Ils ont donc `cap_add: NET_ADMIN`, l'accès à `/dev/net/tun` et `net.ipv4.ip_forward=1`. L'entrypoint (`scripts/agent-entrypoint.sh`) essaie le module kernel de l'hôte ; s'il n'existe pas (Docker Desktop macOS/Windows), il bascule sur `wireguard-go` en userspace. Dans les deux cas l'agent voit une interface `wg0` normale.

Les fichiers concernés :

```
backend/
├── docker-compose.yml           la stack
├── Dockerfile.api               image de l'API (alpine, utilisateur non-root)
├── Dockerfile.agent             image de l'agent (wireguard-tools + wireguard-go)
├── scripts/agent-entrypoint.sh  crée wg0 puis exec wg-agent
├── seed/000001_seed_servers.*   les 3 serveurs
└── .dockerignore
```

## 1.2 Prérequis

- Docker Desktop (macOS / Windows) ou Docker Engine + Compose v2 (Linux)
- Ports libres sur l'hôte : `8080/tcp`, `5432/tcp`, `51820-51822/udp`

Aucun outil Go, `protoc` ou WireGuard n'est requis sur la machine : tout se compile dans les images.

## 1.3 Démarrer

```bash
cd backend
make up            # = docker compose up -d --build
make ps
```

Ordre de démarrage garanti par `depends_on` : `db` (healthy) → `migrate` → `seed-servers` → `api` → agents.

Vérifier :

```bash
curl -s localhost:8080/health
make logs          # api + les 3 agents
```

Dans les logs de l'API, toutes les 10 s (`AGENT_SYNC_INTERVAL` est réduit en local) :

```
reconciler: Paris #1 (local) (agent-fr:50051): 0 peers, 0 online in 4ms
reconciler: New York #1 (local) (agent-us:50051): 0 peers, 0 online in 3ms
reconciler: Tokyo #1 (local) (agent-jp:50051): 0 peers, 0 online in 3ms
```

Dans les logs d'un agent :

```
[entrypoint] kernel module unavailable, starting wireguard-go (userspace)
[entrypoint] wg0 up: 10.8.0.1/24 udp/51820 public-key orLSSB…
agent listening on [::]:50051 (interface=wg0 backend=wgctrl tls=false)
```

## 1.4 Tester le parcours complet

```bash
# 1. Compte + token
curl -s -X POST localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"me@test.dev","password":"secret123"}'
TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"me@test.dev","password":"secret123"}' | jq -r .token)

# 2. Les 3 régions sont là
curl -s localhost:8080/api/v1/server/list -H "Authorization: Bearer $TOKEN" | jq '.[] | {region, grpc_endpoint, subnet}'

# 3. État de l'agent FR (id fixé par le seed)
FR=11111111-1111-4111-8111-111111111111
curl -s localhost:8080/api/v1/server/$FR/agent/status -H "Authorization: Bearer $TOKEN" | jq

# 4. Créer un device sur FR → l'API pousse le peer à agent-fr
curl -s -X POST localhost:8080/api/v1/device \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "{\"server_id\":\"$FR\",\"name\":\"laptop\"}" | jq '{assigned_ip, agent_synced, config}'

# 5. Le peer est bien sur wg0 du conteneur
docker compose exec agent-fr wg show wg0
curl -s localhost:8080/api/v1/server/$FR/agent/peers -H "Authorization: Bearer $TOKEN" | jq
```

`agent_synced: true` confirme que le push gRPC a réussi immédiatement. Si l'agent était arrêté, la réponse dirait `false` et le réconciliateur appliquerait le peer au tick suivant.

### Tester le tunnel lui-même

La `config` renvoyée à l'étape 4 est un fichier wg-quick complet, importable dans l'app WireGuard officielle (desktop ou mobile). Deux choses à savoir :

- **Depuis la machine hôte**, l'`Endpoint` `127.0.0.1:51820` fonctionne tel quel.
- **Depuis un téléphone sur le même Wi-Fi**, il faut l'IP LAN de la machine. Mettre à jour le serveur une fois :

  ```bash
  curl -s -X PUT localhost:8080/api/v1/server/$FR \
    -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -d '{"public_endpoint":"192.168.1.20:51820"}'
  ```

  et recréer le device (la config embarque l'endpoint).

Une fois connecté, `wg show wg0` dans le conteneur affiche un `latest handshake` ; au tick suivant, `device.last_handshake` est renseigné et `online_peers` passe à 1 dans `/agent/status`.

## 1.5 Commandes utiles

```bash
make logs                              # suivre api + agents
make down                              # arrêter, garder la base
make reset                             # arrêter + supprimer le volume (re-migre et re-seed au prochain up)
docker compose restart agent-us        # simuler une panne d'agent
docker compose exec agent-jp wg show   # inspecter une interface
docker compose exec db psql -U postgres wg-vpn-platform -c 'select name, current_peers from server'
```

## 1.6 Variables

Le `docker-compose.yml` a des valeurs par défaut pour tout. Surchargeable via l'environnement ou un fichier `.env` à côté :

| Variable | Défaut | Rôle |
|----------|--------|------|
| `AGENT_TOKEN` | `dev-agent-token` | Secret partagé API ↔ agents (injecté aux deux) |
| `JWT_SECRET` | `dev_jwt_secret_change_me` | Signature des JWT |

Les clés WireGuard des 3 agents sont **fixes et en clair** dans le compose (leurs clés publiques sont dans le seed). C'est ce qui permet au seed de connaître `wg_public_key` avant que les agents démarrent. Ne jamais réutiliser ces clés ailleurs.

## 1.7 Dépannage local

| Symptôme | Cause probable | Correction |
|----------|----------------|------------|
| `agent-*` redémarre en boucle, log `interface wg0: … not found` | `/dev/net/tun` absent ou `NET_ADMIN` refusé | Vérifier que Docker Desktop est à jour ; sur Linux, `modprobe tun` |
| `ip link add wg0 type wireguard` échoue **et** `wireguard-go` aussi | pas de `/dev/net/tun` | idem |
| API : `Unauthenticated: invalid token` | `AGENT_TOKEN` différent entre `api` et un agent | Même valeur partout, `make down && make up` |
| API : `ErrAgentUnavailable … agent-fr:50051` | agent pas encore démarré | Normal pendant ~2 s au boot ; sinon `docker compose logs agent-fr` |
| `seed-servers` échoue sur `duplicate key … wg_public_key` | un serveur avec la même clé existe déjà (créé à la main) | `make reset` ou supprimer le doublon |
| Le tunnel se connecte mais pas d'Internet | NAT non appliqué dans le conteneur | Voir log `[entrypoint] NAT …` ; en userspace sur Docker Desktop c'est parfois limité — le sous-réseau VPN reste joignable |

---

# Partie 2 — Production (plusieurs VPS)

## 2.1 Topologie

```
 VPS « control plane »                        VPS « région » (× N)
 api.example.com                              vpn-fr.example.com
┌──────────────────────────┐                ┌────────────────────────────┐
│ nginx/caddy :443 ──▶ api │   tcp 50051    │ wg-agent :50051  (TLS)     │
│ PostgreSQL               │ ──gRPC+TLS───▶ │   ↳ wg0 (kernel)           │
│ .env : AGENT_TOKEN,      │   + token      │ udp 51820 ◀── clients VPN  │
│        AGENT_TLS_CA      │                │ firewall : 50051 ← IP API  │
└──────────────────────────┘                └────────────────────────────┘
```

L'**API initie** toutes les connexions ; l'agent ne fait qu'écouter. Il n'y a donc rien à configurer côté agent pour « trouver » l'API — seulement un port ouvert, un token et un certificat.

Trois choses changent par rapport au local :

1. `grpc_endpoint` devient l'IP publique / le DNS du VPS région.
2. **TLS est obligatoire** : sans lui, le token circule en clair sur Internet.
3. Le port 50051 est **filtré par IP source** : seul le VPS API peut le joindre.

## 2.2 Choix du TLS

| | Let's Encrypt | CA privée |
|---|---|---|
| Prérequis | un nom DNS par agent, port 80 ouvert pour la validation | aucun |
| Côté API | `AGENT_TLS_CA` vide (CA système) | `AGENT_TLS_CA=/etc/wg-api/ca.crt` |
| Renouvellement | automatique (certbot), redémarrer l'agent après | manuel, certs longs (2 ans) |
| Adapté à | peu de serveurs, DNS déjà en place | beaucoup de serveurs, ou IP nues |

Le reste du guide montre la **CA privée** (plus autonome). Pour Let's Encrypt, remplacer l'étape 2.4.C par `certbot certonly --standalone -d vpn-fr.example.com` et pointer `AGENT_TLS_CERT/KEY` vers `/etc/letsencrypt/live/<domaine>/fullchain.pem` et `privkey.pem`.

## 2.3 VPS control plane

### A. Secrets

```bash
sudo mkdir -p /etc/wg-api && cd /etc/wg-api

# Token partagé avec tous les agents
openssl rand -base64 48 | tr -d '\n' | sudo tee agent.token; echo

# CA privée (garder ca.key hors ligne après avoir signé les agents)
sudo openssl req -x509 -newkey ed25519 -nodes -days 3650 \
  -keyout ca.key -out ca.crt -subj "/CN=wg-agents-ca"
sudo chmod 600 ca.key agent.token
```

### B. PostgreSQL + migrations

```bash
sudo -u postgres createuser wgvpn -P
sudo -u postgres createdb -O wgvpn wg-vpn-platform
cd /opt/wg-vpn-platform/backend
DB_USER=wgvpn DB_PASSWORD=… make migrate-up
```

(Ne pas lancer `make seed-up` en production : les serveurs sont déclarés via l'API à l'étape 2.5.)

### C. L'API

```bash
make build                       # bin/wg-vpn-platform
sudo install bin/wg-vpn-platform /usr/local/bin/wg-api
```

`/etc/wg-api/env` :

```ini
APP_ENV=production
APP_PORT=8080

DB_HOST=127.0.0.1
DB_USER=wgvpn
DB_PASSWORD=…
DB_NAME=wg-vpn-platform
DB_SSLMODE=disable          # require si PostgreSQL est sur une autre machine

JWT_SECRET=<openssl rand -base64 48>
JWT_EXPIRATION_HOURS=2160

WG_DNS=1.1.1.1,8.8.8.8
WG_ALLOWED_IPS=0.0.0.0/0,::/0
WG_KEEPALIVE=25
WG_MTU=1280                 # MTU du tunnel client ; 1280 évite la fragmentation partout
WG_IPV6_PREFIX=fd7a:5f1c:9e42::/64   # IPv6 dans le tunnel (« off » pour désactiver) ; identique sur les serveurs VPN

# Lien vers les agents
AGENT_TOKEN=<contenu de /etc/wg-api/agent.token>
AGENT_TLS_CA=/etc/wg-api/ca.crt
AGENT_CALL_TIMEOUT=10s
AGENT_SYNC_ENABLED=true
AGENT_SYNC_INTERVAL=30s
```

`/etc/systemd/system/wg-api.service` :

```ini
[Unit]
Description=WG VPN control plane
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
User=wgapi
Group=wgapi
EnvironmentFile=/etc/wg-api/env
ExecStart=/usr/local/bin/wg-api
Restart=always
RestartSec=3
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadOnlyPaths=/etc/wg-api

[Install]
WantedBy=multi-user.target
```

```bash
sudo useradd -r -s /usr/sbin/nologin wgapi
sudo systemctl enable --now wg-api
curl -s localhost:8080/health
```

### D. Reverse proxy HTTPS

L'API écoute en HTTP sur 8080 ; exposer `https://api.example.com` via Caddy (TLS automatique) :

```
api.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

ou nginx + certbot. En `APP_ENV=production`, le middleware CORS de dev est désactivé ; l'app mobile utilise `EXPO_PUBLIC_API_URL=https://api.example.com/api/v1`.

### E. Firewall

```bash
ufw allow 22/tcp
ufw allow 80,443/tcp
ufw enable
```

Rien d'autre : PostgreSQL et le port 8080 restent locaux.

## 2.4 VPS région (répéter pour chaque serveur)

### A. WireGuard

```bash
cd /opt/wg-vpn-platform/backend/scripts
sudo WG_SUBNET=10.8.0.0/24 WG_PORT=51820 ./setup-wg-server.sh
```

Le script installe `wireguard-tools`, génère la clé du serveur, écrit `/etc/wireguard/wg0.conf` (adresse passerelle `.1`, NAT, forwarding), active `wg-quick@wg0`, et **affiche la clé publique** — la noter pour l'étape 2.5.

Chaque région doit avoir un **subnet différent** si un même utilisateur peut avoir des devices sur plusieurs serveurs ; ce n'est pas techniquement obligatoire (l'unicité d'IP est par serveur) mais évite les confusions de routage côté client.

### B. L'agent

Compiler sur la machine de build (l'agent est un binaire statique Linux) :

```bash
make build-agent                                  # bin/wg-agent (linux/amd64)
GOARCH=arm64 make build-agent                     # pour un VPS ARM
scp bin/wg-agent root@vpn-fr.example.com:/usr/local/bin/
```

### C. Certificat de l'agent (signé par la CA privée)

Sur la machine qui détient `ca.key` :

```bash
NAME=vpn-fr; IP=203.0.113.10; DNS=vpn-fr.example.com
openssl req -newkey ed25519 -nodes -keyout $NAME.key -out $NAME.csr -subj "/CN=$NAME"
openssl x509 -req -in $NAME.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out $NAME.crt -days 825 \
  -extfile <(printf "subjectAltName=IP:%s,DNS:%s\nextendedKeyUsage=serverAuth" $IP $DNS)
scp $NAME.crt $NAME.key root@$DNS:/etc/wg-agent/
```

Le SAN doit contenir exactement ce que l'API met dans `grpc_endpoint` (IP ou DNS, sans le port). Sinon, positionner `AGENT_TLS_SERVER_NAME` côté API.

### D. Configuration et service

`/etc/wg-agent/env` :

```ini
AGENT_LISTEN=0.0.0.0:50051
WG_INTERFACE=wg0
AGENT_BACKEND=wgctrl
AGENT_TOKEN=<le même que l'API>
AGENT_TLS_CERT=/etc/wg-agent/vpn-fr.crt
AGENT_TLS_KEY=/etc/wg-agent/vpn-fr.key
```

```bash
sudo chmod 600 /etc/wg-agent/env /etc/wg-agent/*.key
```

`/etc/systemd/system/wg-agent.service` :

```ini
[Unit]
Description=WireGuard agent
After=network-online.target wg-quick@wg0.service
Requires=wg-quick@wg0.service

[Service]
EnvironmentFile=/etc/wg-agent/env
ExecStart=/usr/local/bin/wg-agent
Restart=always
RestartSec=3
# wgctrl a besoin de NET_ADMIN pour configurer wg0, rien de plus.
AmbientCapabilities=CAP_NET_ADMIN
CapabilityBoundingSet=CAP_NET_ADMIN
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now wg-agent
journalctl -u wg-agent -f
# → agent listening on [::]:50051 (interface=wg0 backend=wgctrl tls=true)
```

Si l'agent refuse de démarrer avec `interface wg0: … not found`, c'est que `wg-quick@wg0` n'est pas up : `systemctl status wg-quick@wg0`.

### E. Firewall

```bash
ufw allow 22/tcp
ufw allow 51820/udp                                        # clients WireGuard
ufw allow from <IP_PUBLIQUE_VPS_API> to any port 50051 proto tcp   # gRPC, API uniquement
ufw enable
```

C'est la règle la plus importante de tout le déploiement : même si le token fuyait, seul le VPS API peut parler à l'agent.

## 2.5 Déclarer la région dans l'API

Depuis n'importe où, avec un JWT valide :

```bash
curl -s -X POST https://api.example.com/api/v1/server \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{
    "region": "fr",
    "name": "Paris #1",
    "public_endpoint": "vpn-fr.example.com:51820",
    "wg_public_key": "<clé affichée par setup-wg-server.sh>",
    "grpc_endpoint": "vpn-fr.example.com:50051",
    "subnet": "10.8.0.0/24",
    "max_peers": 200
  }'
```

Vérifier immédiatement :

```bash
curl -s https://api.example.com/api/v1/server/<id>/agent/status -H "Authorization: Bearer $TOKEN"
# {"health":{"interface":"wg0","backend":"wgctrl","interface_up":true,...},"interface":{...}}
```

Et dans `journalctl -u wg-api`, au tick suivant : `reconciler: Paris #1 (vpn-fr.example.com:50051): 0 peers, 0 online`.

Le serveur est en service : les devices créés dessus sont poussés à l'agent en temps réel, et resynchronisés toutes les 30 s.

## 2.6 Check-list de sécurité

- [ ] `AGENT_TOKEN` ≥ 32 octets aléatoires, identique partout, jamais commité
- [ ] TLS actif sur chaque agent (`tls=true` dans ses logs) et `AGENT_TLS_CA` côté API
- [ ] Port 50051 filtré par IP source sur chaque VPS région
- [ ] `ca.key` stocké hors des serveurs après signature
- [ ] `JWT_SECRET` fort ; `APP_ENV=production` (désactive CORS permissif et le mode debug Gin)
- [ ] PostgreSQL non exposé (écoute `127.0.0.1` ou `DB_SSLMODE=require` si distant)
- [ ] Les services tournent sous des utilisateurs dédiés (`wgapi`) ou avec capabilities minimales (`CAP_NET_ADMIN` seulement pour l'agent)

---

# Partie 3 — Exploitation

## 3.1 Ajouter une région

Répéter la Partie 2.4 (WireGuard, agent, cert, firewall) puis 2.5 (`POST /server`). Aucun redémarrage de l'API : le réconciliateur découvre le serveur au tick suivant, et le pool ouvre la connexion au premier appel.

## 3.2 Retirer une région

```bash
# 1. Bloquer les nouveaux devices, laisser les existants
PUT /server/<id>  {"is_active": false}
# 2. Migrer ou supprimer les devices (PUT /device/<id> {"server_id": "<autre>"} réalloue l'IP)
# 3. Supprimer
DELETE /server/<id>
```

Un serveur inactif n'est plus réconcilié ; ses endpoints `/agent/*` restent disponibles pour diagnostic.

## 3.3 Rotation du token

Le token est global. Rotation sans coupure :

1. Générer le nouveau token.
2. Sur **chaque agent** : mettre à jour `/etc/wg-agent/env`, `systemctl restart wg-agent`. Pendant cette phase l'API échoue sur cet agent (`Unauthenticated`) — les devices créés sont marqués `agent_synced: false` et rattrapés ensuite.
3. Sur l'API : mettre à jour `/etc/wg-api/env`, `systemctl restart wg-api`. Le premier `RunOnce` remet tout le monde d'accord.

Pour une rotation vraiment sans fenêtre d'erreur, il faudrait un token par serveur en base — voir les suites dans `AGENT_GRPC.md`.

## 3.4 Renouveler un certificat d'agent

Re-signer (2.4.C), remplacer les fichiers, `systemctl restart wg-agent`. L'API se reconnecte automatiquement (backoff gRPC, quelques secondes).

## 3.5 Diagnostic

| Ce que vous voyez | Où regarder | Signification |
|-------------------|-------------|---------------|
| `GET /server/:id/agent/status` → `503` | `journalctl -u wg-agent` sur le VPS région | agent arrêté, port filtré, ou mauvais DNS dans `grpc_endpoint` |
| `503` avec `Unauthenticated` | les deux fichiers `env` | tokens différents |
| `503` avec `x509: certificate …` | SAN du cert vs `grpc_endpoint` | nom/IP ne correspond pas ; `AGENT_TLS_SERVER_NAME` ou re-signer |
| `interface_up: false` | `systemctl status wg-quick@wg0` | `wg0` down sur le serveur |
| `agent_synced: false` à la création | logs API `push peer … failed` | agent injoignable à cet instant ; corrigé au tick suivant |
| `current_peers` ne bouge pas | logs API `reconciler:` | réconciliateur désactivé ou serveur `is_active=false` |
| Client connecté mais `last_handshake` vide | `wg show wg0` sur le serveur | si handshake présent : attendre un tick ; sinon problème client (endpoint, clé) |

Commandes sur un VPS région :

```bash
wg show wg0                       # peers, handshakes, trafic
journalctl -u wg-agent -n 50      # appels reçus, erreurs
ss -ltnp | grep 50051             # l'agent écoute ?
```

## 3.6 Mises à jour

**API** : `make build`, remplacer le binaire, `systemctl restart wg-api`. Les migrations (`make migrate-up`) avant le redémarrage si le schéma change. Coupure HTTP de quelques secondes ; les tunnels WireGuard existants ne sont **pas** affectés (ils vivent sur les serveurs région).

**Agent** : remplacer le binaire, `systemctl restart wg-agent`. Coupure gRPC de quelques secondes, `wg0` et les tunnels restent up (l'interface appartient à `wg-quick`, pas à l'agent). Au redémarrage, le premier `SyncPeers` du réconciliateur garantit que l'interface est cohérente avec la base.

**Contrat gRPC** : si `agent.proto` change, régénérer (`make proto`), déployer les agents **avant** l'API si on ajoute des RPC (un vieil agent répond `Unimplemented` aux nouveaux appels), ou l'API avant si on en retire.
