# Module `wireguard`

Ce package regroupe toute la logique WireGuard de la plateforme, **indépendamment de la base de données et de l'API HTTP**. Il ne connaît ni `User`, ni `Device`, ni `Server` : il manipule uniquement des clés, des adresses IP, des fichiers de configuration et des peers. Ce sont les packages `device` et `vpn-server` qui l'utilisent.

```
internal/wireguard/
├── keys.go       Génération et validation des clés Curve25519
├── ipam.go       Allocation d'adresses IP dans un subnet (IPAM)
├── config.go     Rendu des fichiers .conf (client et serveur)
├── settings.go   Paramètres globaux (DNS, AllowedIPs, keepalive…)
├── peer.go       Représentation d'un peer et de son état
├── client.go     Pilotage d'une interface WireGuard (wgctrl + mémoire)
└── *_test.go     Tests unitaires
```

---

## 1. Rappels WireGuard

Pour comprendre le code, il faut avoir en tête quatre notions.

**Clés.** Chaque participant (serveur ou appareil) possède une paire de clés Curve25519 : une **clé privée** (secrète, 32 octets) et une **clé publique** dérivée de la privée. Elles sont échangées encodées en **base64** (44 caractères). Un participant s'identifie uniquement par sa clé publique.

**Peer.** Du point de vue d'une interface WireGuard, tout autre participant est un *peer*. Pour un appareil, le serveur est son peer ; pour le serveur, chaque appareil est un peer.

**AllowedIPs.** Pour chaque peer, on déclare quelles adresses lui sont associées.
- Côté **serveur**, `AllowedIPs = 10.8.0.5/32` signifie « le trafic vers 10.8.0.5 va à ce peer, et je n'accepte de lui que des paquets provenant de 10.8.0.5 ».
- Côté **client**, `AllowedIPs = 0.0.0.0/0, ::/0` signifie « envoie tout mon trafic dans le tunnel » (full VPN).

**Handshake.** Le client et le serveur négocient une session. La date du dernier handshake permet de savoir si un peer est « en ligne ».

---

## 2. `keys.go` - clés

```go
kp, err := wireguard.GenerateKeyPair()
kp.PrivateKey // "cGxlYXNlIGRvbnQgdXNlIHRoaXMga2V5IGFueXdoZXJlIQ="
kp.PublicKey  // "…"
```

### Génération

`GenerateKeyPair` tire 32 octets aléatoires (`crypto/rand`), puis applique le **clamping** Curve25519 :

```go
key[0]  &= 248   // efface les 3 bits de poids faible
key[31] &= 127   // efface le bit de poids fort
key[31] |= 64    // force le 2e bit de poids fort
```

Ce clamping est imposé par la spécification X25519 : il garantit que la clé privée est un multiple de 8 dans la bonne plage, ce qui neutralise certaines attaques. Sans lui, la clé serait refusée par certaines implémentations.

La clé publique est ensuite dérivée par `curve25519.X25519(priv, Basepoint)` - c'est la même opération que fait `wg pubkey`.

### Validation

`ValidateKey` décode le base64 et vérifie qu'on obtient exactement **32 octets**. C'est tout ce qu'on peut vérifier sur une clé publique : n'importe quelle suite de 32 octets est un point valide sur la courbe. Cette fonction est appelée partout où l'utilisateur fournit une clé (création d'un device ou d'un serveur).

### Autres fonctions

- `PublicKeyFromPrivate` - re-dérive la publique depuis la privée (utile pour vérifier une cohérence).
- `GeneratePresharedKey` - 32 octets aléatoires pour la clé pré-partagée optionnelle, qui ajoute une couche de chiffrement symétrique (résistance post-quantique).

> Ce fichier ne dépend que de `golang.org/x/crypto`, pas de `wgctrl`. L'API centrale peut ainsi générer des clés sans embarquer le code de pilotage kernel.

---

## 3. `ipam.go` - allocation d'IP

IPAM = *IP Address Management*. Chaque appareil doit recevoir une IP **unique** dans le subnet du serveur (par exemple `10.8.0.0/24`). Le `Pool` se charge de trouver la prochaine adresse libre.

```go
pool, _ := wireguard.NewPool("10.8.0.0/24")
pool.Gateway()      // "10.8.0.1"       adresse du serveur dans le tunnel
pool.GatewayCIDR()  // "10.8.0.1/24"    pour la ligne Address= du serveur
pool.Network()      // "10.8.0.0/24"

ip, _ := pool.Allocate([]string{"10.8.0.2", "10.8.0.3"})
// ip == "10.8.0.4"
```

### Adresses réservées

`Allocate` ne distribue jamais :
- l'**adresse réseau** (`10.8.0.0`) - elle désigne le subnet lui-même,
- la **gateway** (`10.8.0.1`) - convention : première adresse utilisable, attribuée au serveur,
- les adresses passées dans `usedIPs`.

Le premier device d'un serveur obtient donc `.2`. C'est exactement ce que vérifie `TestPoolAllocateSkipsGatewayAndUsedIPs`.

### Algorithme

C'est un parcours linéaire : on part de l'adresse réseau, on incrémente octet par octet (`incrementIP` gère le report, comme une addition), et on s'arrête à la première adresse non réservée encore contenue dans le subnet. Pour un `/24` (254 adresses utilisables) c'est instantané ; pour des subnets très larges, une stratégie plus fine serait à envisager.

### Pourquoi `Allocate` reçoit la liste des IP utilisées ?

Le pool est **sans état** : il ne mémorise rien entre deux appels. C'est la base de données qui fait foi sur les IP attribuées. Le service `device` interroge donc la table `device` (`FindAssignedIPsByServer`) puis passe le résultat au pool. Cela évite toute désynchronisation entre un cache mémoire et la base.

### Détails utiles

- `normalizeHostIP("10.8.0.5/32")` → `"10.8.0.5"` - accepte indifféremment une IP nue ou en notation CIDR.
- `hostIPWithCIDR("10.8.0.5")` → `"10.8.0.5/32"` (et `/128` pour une IPv6) - ajoute le préfixe d'hôte requis dans les fichiers de configuration.
- `Contains` vérifie qu'une IP fournie manuellement appartient bien au subnet.

---

## 4. `config.go` - fichiers de configuration

WireGuard se configure avec des fichiers INI simples, lus par `wg-quick`. Ce fichier les génère.

### Config client (`RenderClientConfig`)

C'est ce que l'utilisateur importe dans son application WireGuard :

```ini
[Interface]
PrivateKey = <clé privée de l'appareil>
Address = 10.8.0.5/32
DNS = 1.1.1.1, 8.8.8.8

[Peer]
PublicKey = <clé publique du serveur>
Endpoint = vpn-fr.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
```

- `Address` reçoit systématiquement un `/32` : l'appareil n'a qu'une seule adresse.
- `PersistentKeepalive = 25` envoie un paquet toutes les 25 s pour maintenir le NAT ouvert (indispensable derrière une box ou en 4G).
- Si `PrivateKey` est vide, la constante `PrivateKeyPlaceholder` (`<CLIENT_PRIVATE_KEY>`) est écrite à la place - voir la section « Sécurité des clés privées ».

### Config serveur (`RenderServerConfig`)

Le `wg0.conf` que l'agent posera sur le serveur VPN, avec un bloc `[Peer]` par appareil :

```ini
[Interface]
PrivateKey = <clé privée du serveur>
Address = 10.8.0.1/24
ListenPort = 51820

[Peer]
PublicKey = <clé publique de l'appareil A>
AllowedIPs = 10.8.0.2/32

[Peer]
PublicKey = <clé publique de l'appareil B>
AllowedIPs = 10.8.0.3/32
```

### `SplitList`

Découpe `"1.1.1.1, 8.8.8.8"` en `["1.1.1.1", "8.8.8.8"]` en ignorant les espaces et éléments vides. Sert à lire les variables d'environnement `WG_DNS` et `WG_ALLOWED_IPS`.

---

## 5. `settings.go` - paramètres globaux

`Settings` regroupe ce qui est commun à toute la plateforme et vient du `.env` :

| Champ | Variable | Rôle |
|-------|----------|------|
| `Pool` | `WG_DEFAULT_SUBNET` | Subnet par défaut, utilisé si un serveur est créé sans `subnet` |
| `DNS` | `WG_DNS` | Résolveurs poussés aux clients |
| `AllowedIPs` | `WG_ALLOWED_IPS` | Trafic routé dans le tunnel côté client |
| `Keepalive` | `WG_KEEPALIVE` | Intervalle de keepalive |

`NewSettings` est appelé une fois au démarrage dans `app.go` ; une erreur (subnet invalide) arrête l'application immédiatement plutôt que d'échouer à la première création de device.

`ClientConfig(privateKey, ip, serverPubKey, endpoint)` est un raccourci qui remplit `ClientConfigInput` avec les valeurs globales : le service `device` n'a qu'à fournir ce qui est spécifique à l'appareil et au serveur.

---

## 6. `peer.go` - peers

Trois structures :

- **`Peer`** - la configuration *désirée* d'un peer : clé publique, AllowedIPs, endpoint, PSK, keepalive. C'est ce qu'on envoie à l'interface.
- **`PeerStatus`** - l'état *observé* : `Peer` + date du dernier handshake + octets reçus/émis. C'est ce qu'on lit depuis l'interface.
- **`InterfaceStatus`** - l'état d'une interface complète (`wg0`) et de tous ses peers.

`NewDevicePeer(publicKey, addresses...)` construit le peer serveur d'un appareil avec une entrée hôte par adresse (`ip/32`, `ipv6/128`). Côté API on passe par `Settings.DevicePeer`, qui ajoute l'IPv6 dérivée (`DeviceIPv6` : l'IPv4 du device dans les 32 bits bas de `WG_IPV6_PREFIX`, donc rien à stocker ni à allouer).

`PeerStatus.IsOnline(3 * time.Minute)` répond « oui » si un handshake a eu lieu dans la fenêtre. WireGuard renégocie environ toutes les 2 minutes quand il y a du trafic ; au-delà de 3 minutes sans handshake, le peer est considéré déconnecté.

---

## 7. `client.go` - piloter une interface

C'est la partie « système » du module, destinée à l'**agent** qui tournera sur chaque serveur VPN. L'API centrale ne l'utilise pas directement.

### L'interface `Client`

```go
type Client interface {
    Interface(ctx, iface) (*InterfaceStatus, error)   // lire l'état de wg0
    ListPeers(ctx, iface) ([]PeerStatus, error)        // lister les peers
    AddPeer(ctx, iface, Peer) error                    // ajouter / mettre à jour un peer
    RemovePeer(ctx, iface, publicKey) error            // retirer un peer
    SyncPeers(ctx, iface, []Peer) error                // remplacer tous les peers
    Close() error
}
```

Deux implémentations respectent ce contrat, ce qui permet d'écrire le code de l'agent une seule fois et de le tester sans droits root.

### `WGCtrlClient` - la vraie

S'appuie sur [`wgctrl`](https://github.com/WireGuard/wgctrl-go), la bibliothèque officielle. Elle parle au module kernel (Linux) ou au démon userspace (`wireguard-go`, macOS…) et fait la même chose que la commande `wg`.

Points clés :
- `AddPeer` positionne `ReplaceAllowedIPs = true` : les AllowedIPs sont remplacées, pas ajoutées. Si un device change d'IP, l'ancienne ne traîne pas.
- `SyncPeers` positionne `ReplacePeers = true` : l'interface est mise dans l'état exact de la liste. C'est l'opération idéale pour une synchronisation complète API → agent, ou au redémarrage de l'agent.
- `RemovePeer` envoie un `PeerConfig{Remove: true}`.
- Les conversions `toWGPeerConfig` / `fromWGPeer` traduisent entre nos types (`string`, `[]string`) et ceux de `wgtypes` (`Key`, `net.IPNet`, `*net.UDPAddr`, `time.Duration`).
- `wgctrl` renvoie `os.ErrNotExist` si l'interface n'existe pas ; `wrapDeviceErr` le convertit en `ErrInterfaceNotFound`, plus explicite.

### `MemoryClient` - pour le développement et les tests

Stocke les peers dans une `map[iface]map[publicKey]PeerStatus` protégée par un `sync.RWMutex`. Aucune interface réseau n'est touchée. Il valide quand même les clés (`ValidateKey`) et renvoie `ErrInterfaceNotFound` sur une interface inconnue, pour se comporter comme la vraie implémentation.

`PublicKeys(iface)` est un helper pour les tests.

---

## 8. Intégration dans l'API

### Création d'un device (`device.Service.Create`)

```
POST /api/v1/device  { "server_id": "…", "name": "Mon laptop" }
```

1. `resolveServer` charge le serveur, vérifie `is_active`, construit un `Pool` à partir de `server.Subnet`.
2. **Clé** : si `public_key` est absent, `GenerateKeyPair()` ; sinon `ValidateKey()`.
3. **IP** : si `assigned_ip` est absent, `pickIP` lit les IP déjà prises sur ce serveur puis `pool.Allocate()` ; sinon `pool.Contains()` vérifie qu'elle est dans le subnet.
4. Insertion en base.
5. Réponse : le device, la clé privée (si générée) et la config complète via `Settings.ClientConfig`.

### Téléchargement de la config

```
GET /api/v1/device/:id/config   →  text/plain, fichier .conf
```

Rend la config avec `<CLIENT_PRIVATE_KEY>` en placeholder.

### Multi-serveur

Chaque serveur a son propre `subnet`. L'unicité d'une IP est garantie par `(server_id, assigned_ip)` en base, pas globalement : deux serveurs peuvent utiliser `10.8.0.0/24` sans conflit. Le pool est reconstruit à chaque requête depuis `server.Subnet` - `NewPool` ne fait que parser un CIDR, c'est négligeable.

### Sécurité des clés privées

La plateforme **ne stocke jamais** de clé privée. Quand elle en génère une, elle est renvoyée **une seule fois** dans la réponse de création puis oubliée. Si l'utilisateur perd sa config, il doit recréer le device (ou en fournir une nouvelle clé publique via `PUT`). C'est le même modèle que Tailscale ou Mullvad : une compromission de la base ne donne pas accès aux tunnels.

---

## 9. Tests

```bash
cd backend && go test ./internal/wireguard/ -v
```

| Test | Ce qu'il garantit |
|------|-------------------|
| `TestPoolAllocateSkipsGatewayAndUsedIPs` | `.0` et `.1` réservées, allocation séquentielle, erreur quand le pool est plein |
| `TestPoolContains` | Une IP hors subnet est refusée |
| `TestRenderClientConfig` | Le format du `.conf` client (Address en `/32`, keepalive, etc.) |
| `TestGenerateKeyPairIsValidAndDerivable` | Les clés générées sont valides et cohérentes entre elles |
| `TestValidateKeyRejectsGarbage` | Vide, non-base64 et mauvaise longueur sont rejetés |
| `TestMemoryClientPeerLifecycle` | Add → List → Remove, et erreur sur interface inconnue |

Tout tourne sans base de données ni interface réseau : le module est de la logique pure, ce qui le rend rapide à tester et sûr à modifier.

---

## 10. Pilotage à distance : agent et orchestrateur

Le `Client` de ce module n'est pas utilisé directement par l'API : il tourne dans l'**agent** (`cmd/agent`, `internal/agent`) installé sur chaque serveur VPN. L'API le pilote en **gRPC** via l'**orchestrateur** (`internal/orchestrator`).

```
 API (control plane)                          Serveur VPN (fr-1)
 ┌──────────────────────────┐   gRPC/TLS      ┌──────────────────────────┐
 │ device.Service           │ ──AddPeer────▶  │ agent.Service            │
 │   └─ orchestrator        │ ──RemovePeer──▶ │   └─ wireguard.Client    │
 │ orchestrator.Reconciler  │ ──SyncPeers───▶ │        (WGCtrlClient)    │
 │   (toutes les 30 s)      │ ◀─ListPeers───  │             │            │
 └──────────────────────────┘                 │             ▼            │
                                              │          wg0 (kernel)    │
                                              └──────────────────────────┘
```

- **Contrat** : `api/proto/agent/v1/agent.proto` (code généré dans `gen/agent/v1`, `make proto`). Les conversions proto ↔ `wireguard.Peer` / `PeerStatus` vivent dans `internal/agentproto`.
- **Agent** : expose `Health`, `GetInterface`, `ListPeers`, `AddPeer`, `RemovePeer`, `SyncPeers`. Authentification par token partagé (`AGENT_TOKEN`) et TLS optionnel. `AGENT_BACKEND=memory` permet de le lancer en local sans `wg0`.
- **Orchestrateur** : un client gRPC par `server.grpc_endpoint` (pool de connexions). `device.Service` lui pousse chaque création / modification / suppression ; l'échec est journalisé (`agent_synced: false` dans la réponse) et rattrapé par le réconciliateur.
- **Réconciliateur** : à intervalle régulier, pour chaque serveur actif, `SyncPeers` avec les devices non révoqués de la base, puis `ListPeers` pour alimenter `device.last_handshake` et `server.current_peers`.
- **Endpoints admin** : `GET /server/:id/agent/status`, `GET /server/:id/agent/peers`, `POST /server/:id/agent/sync`.

## 11. Ce qui reste à construire

- **Révocation** : un `POST /device/:id/revoke` renseignant `revoked_at` ; le réconciliateur retirera alors le peer automatiquement, mais un `RemovePeer` immédiat serait préférable.
- **Réallocation** si le `subnet` d'un serveur change alors qu'il a déjà des devices.
- **mTLS** côté agent (aujourd'hui : TLS serveur + token).
