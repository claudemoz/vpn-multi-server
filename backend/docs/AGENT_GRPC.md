# Control plane ↔ agents : la communication gRPC

Ce document explique comment l'API pilote WireGuard sur plusieurs serveurs distants. Il s'adresse à quelqu'un qui connaît le module `internal/wireguard` (voir son README) et veut comprendre **qui parle à qui, quand, et pourquoi c'est construit ainsi**.

```
backend/
├── api/proto/agent/v1/agent.proto   Le contrat (protobuf)
├── gen/agent/v1/                    Code Go généré (ne pas éditer)
├── internal/agentproto/             Conversions proto ↔ types wireguard
├── internal/agent/                  L'agent (serveur gRPC, tourne sur chaque VPN)
├── internal/orchestrator/           Le control plane (client gRPC, tourne dans l'API)
├── cmd/agent/                       Binaire de l'agent
└── scripts/
    ├── gen-proto.sh                 Régénère gen/ depuis le .proto
    └── setup-wg-server.sh           Prépare wg0 sur un serveur neuf
```

---

## 1. Le problème à résoudre

La plateforme a **un** control plane (l'API + PostgreSQL) et **plusieurs** serveurs VPN (Paris, New York…). Quand un utilisateur crée un device, l'API :

1. choisit une IP dans le subnet du serveur,
2. enregistre la clé publique du device en base,
3. **doit configurer un peer sur l'interface `wg0` de ce serveur**, sinon le tunnel ne s'établira jamais.

L'étape 3 se passe sur une autre machine. Il faut donc un processus sur chaque serveur VPN capable de modifier `wg0`, et un protocole pour que l'API lui parle. C'est le rôle de **l'agent** et de **gRPC**.

### Pourquoi gRPC ?

- **Contrat typé** : le `.proto` définit exactement les messages ; le code Go des deux côtés est généré et ne peut pas diverger.
- **Binaire et rapide** : les appels sont de petits messages fréquents (un peer = quelques centaines d'octets).
- **HTTP/2 avec connexions persistantes** : l'API garde une connexion ouverte par serveur et la réutilise ; gRPC gère la reconnexion tout seul si un agent redémarre.
- **Codes d'erreur standardisés** : `Unauthenticated`, `InvalidArgument`, `Unavailable`… l'API sait distinguer « mauvais token » de « agent injoignable » sans parser du texte.

### Qui appelle qui ?

L'**API est le client** et l'**agent est le serveur**. Le champ `server.grpc_endpoint` (`1.2.3.4:50051`) existait déjà dans le modèle : c'est l'adresse à laquelle l'API joint l'agent. Ce choix suppose que l'agent est joignable depuis l'API (port ouvert). L'alternative — l'agent se connecte à l'API — serait meilleure derrière un NAT, mais plus complexe (streams bidirectionnels, gestion des sessions) ; elle n'est pas nécessaire pour des serveurs VPN qui ont, par définition, une IP publique.

---

## 2. Le contrat : `agent.proto`

```protobuf
service AgentService {
  rpc Health(HealthRequest) returns (HealthResponse);
  rpc GetInterface(GetInterfaceRequest) returns (GetInterfaceResponse);
  rpc ListPeers(ListPeersRequest) returns (ListPeersResponse);
  rpc AddPeer(AddPeerRequest) returns (AddPeerResponse);
  rpc RemovePeer(RemovePeerRequest) returns (RemovePeerResponse);
  rpc SyncPeers(SyncPeersRequest) returns (SyncPeersResponse);
}
```

Ce sont exactement les méthodes de l'interface `wireguard.Client`, plus `Health`. L'agent n'invente rien : il **expose sur le réseau** ce que le module `wireguard` sait déjà faire localement.

| RPC | Sémantique | Idempotent ? |
|-----|------------|--------------|
| `Health` | L'agent est vivant ; quelle interface, quel backend, `wg0` est-elle up ? | oui |
| `GetInterface` | État complet de `wg0` : clé publique, port, tous les peers avec leur statut | oui |
| `ListPeers` | Seulement les peers (avec handshake et compteurs) | oui |
| `AddPeer` | Ajoute **ou met à jour** un peer (ses AllowedIPs sont remplacées) | oui |
| `RemovePeer` | Retire un peer par clé publique ; ne se plaint pas s'il n'existe pas | oui |
| `SyncPeers` | Remplace **tout** l'ensemble de peers par la liste fournie | oui |

L'idempotence est volontaire : l'API peut rejouer n'importe quel appel après une coupure sans risque de double effet.

### Les messages

```protobuf
message Peer {                     // configuration désirée
  string public_key = 1;
  string preshared_key = 2;
  string endpoint = 3;             // vide pour un client mobile
  repeated string allowed_ips = 4; // "10.8.0.5/32"
  int32 keepalive = 5;
}

message PeerStatus {               // état observé
  Peer peer = 1;
  google.protobuf.Timestamp last_handshake = 2;  // absent = jamais
  int64 receive_bytes = 3;
  int64 transmit_bytes = 4;
}
```

Même distinction que dans le module `wireguard` : `Peer` = ce qu'on veut, `PeerStatus` = ce qu'on constate. Le `last_handshake` est **omis** (pas mis à zéro) quand aucun handshake n'a eu lieu — `agentproto` fait la traduction avec `time.Time{}` côté Go.

### Génération du code

```bash
make proto          # = scripts/gen-proto.sh
```

Produit `gen/agent/v1/agent.pb.go` (messages) et `agent_grpc.pb.go` (client + serveur). Ces fichiers sont **commités** : on peut builder le projet sans avoir `protoc` installé. On ne les édite jamais à la main ; on modifie le `.proto` et on régénère.

### `internal/agentproto` — le traducteur

Les deux côtés manipulent des `wireguard.Peer` / `wireguard.PeerStatus`, jamais des messages protobuf. `agentproto` contient les fonctions `PeerToProto`, `PeerFromProto`, `PeerStatusToProto`… Ce package est partagé par l'agent et l'orchestrateur ; s'il y avait deux implémentations de la conversion, elles finiraient par diverger.

---

## 3. L'agent : `cmd/agent` + `internal/agent`

Un binaire Go statique, un par serveur VPN, qui tourne en root (ou avec `CAP_NET_ADMIN`) pour pouvoir toucher `wg0`.

```
cmd/agent/main.go     charge la config, construit le Server, attend SIGINT/SIGTERM
internal/agent/
├── config.go         variables d'environnement + validation
├── auth.go           interceptor gRPC : vérifie le bearer token
├── service.go        implémente AgentServiceServer au-dessus de wireguard.Client
└── grpc_server.go    assemble tout : backend WireGuard, TLS, keepalive, arrêt propre
```

### `service.go` — volontairement mince

```go
func (s *Service) AddPeer(ctx, req) (*AddPeerResponse, error) {
    peer := agentproto.PeerFromProto(req.GetPeer())
    // validation minimale : clé valide, au moins une AllowedIP
    if err := s.wg.AddPeer(ctx, s.iface, peer); err != nil {
        return nil, toStatus(err)
    }
    return &agentv1.AddPeerResponse{}, nil
}
```

Chaque RPC fait trois choses : convertir, appeler `wireguard.Client`, convertir le retour. Toute la logique WireGuard reste dans le module `wireguard`, qui est testé indépendamment.

`toStatus` traduit les erreurs du domaine en codes gRPC :

| Erreur Go | Code gRPC | Ce que l'API en déduit |
|-----------|-----------|------------------------|
| `wireguard.ErrInterfaceNotFound` | `FailedPrecondition` | `wg0` n'existe pas sur ce serveur |
| `wireguard.ErrInvalidKey` | `InvalidArgument` | bug côté API, pas la peine de réessayer |
| autre | `Internal` | problème système sur l'agent |

### Deux backends

`AGENT_BACKEND` choisit l'implémentation de `wireguard.Client` :

- **`wgctrl`** (défaut) : le vrai `WGCtrlClient`, parle au kernel. Au démarrage, l'agent vérifie que l'interface existe et **refuse de démarrer** sinon — mieux vaut un échec immédiat au déploiement qu'un agent qui répond `FailedPrecondition` à chaque appel.
- **`memory`** : le `MemoryClient`, tout en RAM. Permet de lancer l'agent sur un Mac de dev sans root ni WireGuard, et de tester toute la chaîne gRPC.

```bash
make run-agent                      # memory, pour développer
AGENT_TOKEN=xxx ./bin/wg-agent      # wgctrl, en prod
```

### `auth.go` — le token partagé

L'agent est un service **privilégié** exposé sur Internet : n'importe qui pouvant l'appeler peut ajouter son propre peer et entrer dans le VPN. Il faut donc authentifier l'API.

Le mécanisme est un **secret partagé** (`AGENT_TOKEN`) envoyé dans les métadonnées gRPC (l'équivalent des headers HTTP) :

```
authorization: Bearer <token>
```

L'interceptor vérifie ce header **avant** que la requête atteigne `service.go`. Points d'attention :

- Comparaison via `crypto/subtle.ConstantTimeCompare` : le temps de réponse ne dépend pas du nombre de caractères corrects, ce qui empêche de deviner le token octet par octet.
- Un `AGENT_TOKEN` vide fait **échouer le démarrage**. Pour le développement local uniquement, `AGENT_INSECURE_NO_AUTH=true` désactive la vérification avec un avertissement dans les logs.

### TLS

Sans TLS, le token circule en clair. `AGENT_TLS_CERT` + `AGENT_TLS_KEY` activent TLS côté agent ; l'API vérifie le certificat avec `AGENT_TLS_CA`. En développement, les deux côtés fonctionnent en clair et le préviennent dans les logs.

> Pourquoi pas mTLS (certificat client aussi) ? C'est l'étape suivante logique et elle remplacerait le token. On a commencé par le token parce qu'il se déploie en une variable d'environnement, sans PKI.

### Arrêt propre

`Serve(ctx)` écoute jusqu'à l'annulation du contexte (SIGINT/SIGTERM), puis `GracefulStop()` laisse les appels en cours se terminer, avec un plafond de `AGENT_SHUTDOWN_TIMEOUT` (10 s) avant un arrêt forcé. Le `wireguard.Client` est fermé en dernier.

---

## 4. Le control plane : `internal/orchestrator`

Côté API, ce package est le seul à connaître gRPC. Le reste de l'application (`device`, `vpn-server`) ne voit que des interfaces Go.

```
internal/orchestrator/
├── client.go        AgentClient : wrapper typé d'un client gRPC + Dialer (TLS, token)
├── pool.go          une connexion par endpoint, réutilisée
├── orchestrator.go  les opérations métier : AddPeer, RemovePeer, SyncServer, Status…
├── reconciler.go    la boucle de fond qui resynchronise tous les serveurs
├── adapters.go      branche vpnserver.Service et device.Repository sur l'orchestrateur
├── handler.go       endpoints HTTP /server/:id/agent/*
└── router.go
```

### `client.go` — un client qui parle `wireguard.*`

```go
client, _ := dialer.Dial("1.2.3.4:50051")
err := client.AddPeer(ctx, wireguard.NewDevicePeer(pubKey, "10.8.0.5"))
```

`AgentClient` cache le protobuf derrière des signatures identiques à `wireguard.Client`. Avant chaque appel, `prepare()` :

1. ajoute le header `authorization: Bearer …`,
2. pose un **deadline** (`AGENT_CALL_TIMEOUT`, 10 s) — un agent qui ne répond pas ne doit pas bloquer une requête HTTP indéfiniment.

Après chaque appel, `wrap()` normalise les erreurs : `Unavailable`, `DeadlineExceeded` et `Canceled` deviennent `ErrAgentUnavailable`. Le reste du code n'a qu'un `errors.Is` à faire pour savoir si c'est « l'agent est down » ou « autre chose ».

Le `Dialer` porte la configuration commune (token, TLS) : on le crée une fois dans `app.go` et il fabrique tous les clients.

### `pool.go` — une connexion par serveur

`grpc.NewClient` est **paresseux** : il ne se connecte qu'au premier appel, et se reconnecte seul si la connexion tombe. Le `Pool` garde donc un `AgentClient` par `grpc_endpoint` dans une map protégée par mutex ; le premier appel vers un serveur le crée, les suivants le réutilisent. Le keepalive HTTP/2 (30 s) maintient la connexion chaude et détecte un agent mort.

### `orchestrator.go` — les opérations

L'orchestrateur dépend de deux interfaces qu'il définit lui-même :

```go
type ServerSource interface {
    Target(id) (Target, error)            // un serveur → son endpoint
    ActiveTargets() ([]Target, error)     // tous les serveurs actifs
    SetCurrentPeers(id, count) error      // écrit server.current_peers
}

type PeerSource interface {
    DesiredPeers(serverID) ([]wireguard.Peer, error)      // devices → peers
    RecordHandshakes(serverID, []wireguard.PeerStatus) error  // écrit device.last_handshake
}
```

`adapters.go` les implémente au-dessus de `vpnserver.Service` et `device.Repository`. Cette indirection sert à deux choses : les tests remplacent la base par des fakes, et l'orchestrateur reste indépendant du schéma SQL.

**`AddPeer` / `RemovePeer`** : trouve l'endpoint du serveur, prend le client dans le pool, appelle. C'est ce qu'utilise `device.Service`.

**`SyncServer`** est l'opération centrale :

```
1. desired  = PeerSource.DesiredPeers(serverID)     ← devices non révoqués en base
2. count    = agent.SyncPeers(desired)              ← wg0 devient exactement `desired`
3. statuses = agent.ListPeers()                     ← handshakes, octets
4. PeerSource.RecordHandshakes(statuses)            ← device.last_handshake
5. ServerSource.SetCurrentPeers(count)              ← server.current_peers
```

Un seul appel et le serveur est dans l'état voulu **quel que soit** son état précédent : peers ajoutés à la main, agent redémarré avec une interface vide, push raté pendant une panne… tout converge.

**`Status`** : `Health` puis, si l'interface est up, `GetInterface`. Compte les peers « en ligne » (`IsOnline(3 min)`).

### `reconciler.go` — la boucle de fond

```go
func (r *Reconciler) Run(ctx) {
    r.RunOnce(ctx)                 // dès le démarrage de l'API
    for chaque tick (AGENT_SYNC_INTERVAL, 30 s) {
        r.RunOnce(ctx)
    }
}
```

`RunOnce` lance un `SyncServer` **par serveur actif, en parallèle** (une goroutine chacun, `WaitGroup`). Un serveur injoignable est loggé et n'empêche pas les autres.

C'est le **filet de sécurité** du système. Sans lui, un agent qui rate un `AddPeer` (redémarrage, réseau) garderait un device fantôme jusqu'à la prochaine action humaine. Avec lui, l'écart dure au plus 30 secondes. C'est aussi la **seule source** de `device.last_handshake` : c'est lui qui permet à l'API de dire « ce device est connecté ».

Le réconciliateur est démarré par `App.Start(ctx)` dans `main.go` et arrêté à la réception du signal d'arrêt, avant le serveur HTTP. `AGENT_SYNC_ENABLED=false` le désactive (utile en test ou si aucun agent n'existe encore).

---

## 5. Où ça se branche dans l'API

### `device.Service` — pousser sans bloquer

```go
type PeerPublisher interface {           // défini dans le package device
    AddPeer(ctx, serverID, wireguard.Peer) error
    RemovePeer(ctx, serverID, publicKey string) error
}
```

`device.Service` reçoit un `PeerPublisher` ; c'est l'orchestrateur qui l'implémente. L'interface est déclarée **côté `device`** et non côté `orchestrator` : ainsi `device` n'importe pas `orchestrator` (qui, lui, importe `device` pour lire le repository) — pas de cycle. Si on passe `nil`, un `noopPublisher` prend le relais : l'API fonctionne sans agent, elle ne fait que persister.

| Action | Ce qui est poussé |
|--------|-------------------|
| `Create` | `AddPeer(pubKey, ip/32)` sur le serveur du device |
| `Update` | si clé ou serveur change : `RemovePeer` de l'ancien **puis** `AddPeer` du nouveau ; si seule l'IP change : `AddPeer` suffit (les AllowedIPs sont remplacées) |
| `Delete` | `RemovePeer` |

**Le push est "best effort"** : un échec est loggé, la requête HTTP réussit quand même. Raisonnement : la base est la source de vérité et le réconciliateur corrigera dans les 30 s. Faire échouer la création du device parce qu'un agent redémarre pénaliserait l'utilisateur pour un problème transitoire. En contrepartie, la réponse de `POST /device` contient `agent_synced: true|false` pour que le client mobile puisse prévenir : « tunnel disponible dans quelques instants ».

### Endpoints d'administration

Sous `/api/v1/server/:id/agent/`, protégés par JWT comme le reste :

| Route | Usage |
|-------|-------|
| `GET  …/agent/status` | health de l'agent + état de `wg0` + nombre de peers en ligne |
| `GET  …/agent/peers`  | les peers réellement configurés, avec handshake et trafic |
| `POST …/agent/sync`   | forcer une réconciliation immédiate, renvoie `{peer_count, online_peers, duration}` |

Codes HTTP : `400` si le serveur n'a pas de `grpc_endpoint`, `503` si l'agent est injoignable (`apperrors.NewUnavailable`), `404` si le serveur n'existe pas.

---

## 6. Configuration

### Côté API (`.env`)

| Variable | Défaut | Rôle |
|----------|--------|------|
| `AGENT_TOKEN` | — | Secret partagé ; **doit être identique** sur tous les agents |
| `AGENT_TLS_CA` | vide | CA des certificats agents ; vide = clair |
| `AGENT_TLS_SERVER_NAME` | vide | Nom attendu dans le certificat (si différent de l'endpoint) |
| `AGENT_DIAL_TIMEOUT` | `5s` | Réservé (la connexion est paresseuse, c'est le call timeout qui borne) |
| `AGENT_CALL_TIMEOUT` | `10s` | Deadline de chaque RPC |
| `AGENT_SYNC_ENABLED` | `true` | Active le réconciliateur |
| `AGENT_SYNC_INTERVAL` | `30s` | Période de réconciliation |

### Côté agent (environnement du process)

| Variable | Défaut | Rôle |
|----------|--------|------|
| `AGENT_LISTEN` | `:50051` | Adresse d'écoute gRPC |
| `WG_INTERFACE` | `wg0` | Interface gérée |
| `AGENT_BACKEND` | `wgctrl` | `wgctrl` ou `memory` |
| `AGENT_TOKEN` | — | Obligatoire (sauf `AGENT_INSECURE_NO_AUTH=true`) |
| `AGENT_TLS_CERT` / `AGENT_TLS_KEY` | vide | Active TLS |
| `AGENT_SHUTDOWN_TIMEOUT` | `10s` | Délai d'arrêt gracieux |

---

## 7. Déployer un nouveau serveur VPN

```bash
# 1. Sur le serveur (root) : crée wg0, active le forwarding et le NAT
WG_SUBNET=10.9.0.0/24 ./scripts/setup-wg-server.sh
# → affiche la clé publique de wg0

# 2. Ouvrir udp/51820 (WireGuard) et tcp/50051 (agent, restreint à l'IP de l'API)

# 3. Déposer et lancer l'agent
make build-agent                       # bin/wg-agent (linux/amd64, statique)
scp bin/wg-agent vpn-us:/usr/local/bin/
AGENT_TOKEN=… WG_INTERFACE=wg0 /usr/local/bin/wg-agent   # via systemd en pratique

# 4. Déclarer le serveur dans l'API
POST /api/v1/server
{
  "region": "us", "name": "New York #1",
  "public_endpoint": "vpn-us.example.com:51820",
  "wg_public_key": "<clé affichée à l'étape 1>",
  "grpc_endpoint": "vpn-us.example.com:50051",
  "subnet": "10.9.0.0/24"
}

# 5. Vérifier
GET /api/v1/server/<id>/agent/status
```

Dès l'étape 4, le réconciliateur inclut ce serveur dans sa boucle ; au premier tick il pousse les devices déjà en base (aucun, pour un serveur neuf) et commence à remonter les handshakes.

---

## 8. Scénarios de panne

**L'agent est down quand un utilisateur crée un device.**
`AddPeer` échoue → log + `agent_synced: false`. Le device existe en base. Quand l'agent revient, le prochain `SyncServer` le configure. Délai max : `AGENT_SYNC_INTERVAL`.

**L'agent redémarre et `wg0` est vide** (par exemple `wg-quick down/up`).
Le prochain `SyncPeers` reconstruit tous les peers depuis la base. Aucune donnée n'est perdue : la base est la seule source de vérité, l'agent n'a pas d'état propre.

**Quelqu'un ajoute un peer à la main sur le serveur** (`wg set wg0 peer …`).
Il sera supprimé au prochain `SyncPeers` (`ReplacePeers = true`). C'est voulu : un peer que l'API ne connaît pas est une porte dérobée.

**L'API redémarre.**
Rien à faire : `App.Start` lance un `RunOnce` immédiat, puis les connexions se rouvrent à la demande.

**Le token diffère entre l'API et un agent.**
Chaque appel renvoie `Unauthenticated` ; visible dans les logs du réconciliateur (`agent 1.2.3.4:50051: Unauthenticated: invalid token`) et via `GET …/agent/status` → `503`.

**Un serveur a `is_active = false`.**
Le réconciliateur l'ignore (`ActiveTargets`). `device.Service` refuse d'y créer des devices. Les endpoints `/agent/*` restent utilisables pour diagnostiquer.

---

## 9. Tests

```bash
cd backend && go test ./internal/agentproto/ ./internal/orchestrator/ -v
```

`agentproto` vérifie que les conversions sont sans perte (aller-retour), y compris le cas « pas de handshake ».

`orchestrator` démarre **un vrai agent gRPC en mémoire dans le process de test** (`startAgent`) et le pilote avec le vrai `AgentClient`. Les sources de données sont des fakes. Ce qui est couvert :

| Test | Garantit |
|------|----------|
| `TestAgentRejectsBadToken` | L'interceptor bloque un mauvais token |
| `TestAddAndRemovePeerThroughOrchestrator` | Le push arrive bien dans `wg0` (mémoire) et en repart |
| `TestSyncServerConvergesAgentAndRecordsState` | Un peer périmé est supprimé, `current_peers` et handshakes sont écrits |
| `TestStatusReportsHealthAndInterface` | `Status` agrège `Health` + `GetInterface` |
| `TestServerWithoutEndpoint` | Erreur explicite `ErrNoEndpoint` |
| `TestUnreachableAgentIsReportedAsUnavailable` | Un port fermé devient `ErrAgentUnavailable` |

Aucun test ne requiert root, une interface réseau réelle ou PostgreSQL.

---

## 10. Limites connues et suites possibles

- **Révocation** : `device.revoked_at` est déjà exclu de `DesiredPeers`, donc un device révoqué disparaît au prochain sync ; il manque l'endpoint `POST /device/:id/revoke` et un `RemovePeer` immédiat.
- **mTLS** : remplacer le token par des certificats clients, ou combiner les deux.
- **Sens de connexion inversé** (agent → API) si des serveurs derrière NAT devaient être supportés.
- **Streaming** : un `WatchPeers` en stream côté agent éviterait le polling toutes les 30 s pour les handshakes — utile si le nombre de serveurs devient grand.
- **Métriques** : les compteurs `receive_bytes` / `transmit_bytes` remontent déjà mais ne sont pas stockés ; un historique permettrait des quotas ou des graphes.
