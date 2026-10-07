# Ajouter un serveur VPN

Ce guide explique comment préparer un VPS, y lancer l'agent, et le déclarer dans l'API. Chaque étape dit quoi faire, puis pourquoi.

L'idée est simple. Il y a **une API** (le cerveau) et **un VPS par région** (le serveur VPN). Sur chaque VPS tournent deux choses distinctes :

- **WireGuard** (`wg0`, port UDP 51820) : le tunnel des téléphones et ordinateurs.
- **l'agent** (port TCP 50051) : un petit programme que l'API appelle pour ajouter ou retirer des peers sur `wg0`.

L'agent n'appelle jamais l'API. C'est l'API qui se connecte à lui. Pour qu'elle sache où aller, on enregistre l'adresse de l'agent dans le champ `grpc_endpoint`.

```
téléphone  --UDP 51820-->  wg0          (le tunnel)
API        --TCP 50051-->  agent --> wg0 (la configuration)
```

Pour le détail du protocole, voir [`AGENT_GRPC.md`](AGENT_GRPC.md). Pour le déploiement local et la prod complète, voir [`DEPLOYMENT.md`](DEPLOYMENT.md).

---

## 1. Choisir un secret commun

Génère un mot de passe long, une seule fois, et mets-le des deux côtés sous le nom `AGENT_TOKEN`.

```bash
openssl rand -base64 48
```

**Pourquoi.** L'agent peut modifier le VPN. Sans secret, n'importe qui qui atteint le port 50051 pourrait s'ajouter comme client. L'API envoie ce secret à chaque appel. S'il n'est pas identique des deux côtés, l'agent refuse avec `Unauthenticated`.

---

## 2. Préparer WireGuard sur le VPS

En root, sur le serveur de la région (ici la France, subnet `10.8.0.0/24`) :

```bash
cd /opt/wg-vpn-platform/backend/scripts
sudo WG_SUBNET=10.8.0.0/24 WG_PORT=51820 ./setup-wg-server.sh
```

Note la **clé publique** affichée à la fin.

**Pourquoi.** L'agent ne crée pas l'interface réseau. Il a besoin qu'elle existe déjà. Le script :

- installe les outils WireGuard ;
- crée `/etc/wireguard/wg0.conf` avec une clé privée et l'adresse passerelle `10.8.0.1` ;
- active le transfert IP et le NAT, sinon un client connecté n'a pas Internet ;
- démarre `wg0` et affiche la clé publique.

Cette clé publique est celle que l'application mobile met dans sa config pour reconnaître le serveur. Le subnet doit être **différent sur chaque région** (`10.8.0.0/24`, puis `10.9.0.0/24`, etc.) : c'est dans ce bloc que l'API pioche l'IP de chaque appareil.

---

## 3. Ouvrir seulement les bons ports

```bash
ufw allow 22/tcp
ufw allow 51820/udp
ufw allow from <IP_PUBLIQUE_DE_L_API> to any port 50051 proto tcp
ufw enable
```

**Pourquoi.**

- `51820/udp` est le tunnel. Il doit être ouvert à tout le monde, parce que les clients sont n'importe où.
- `50051/tcp` est l'agent. Seule l'API a le droit de lui parler. Même si le token fuit, une autre machine ne peut pas joindre l'agent.
- Le port 22 reste pour l'administration SSH.

---

## 4. Compiler l'agent et le copier sur le VPS

Sur la machine de développement, dans `backend/` :

```bash
make build-agent
scp bin/wg-agent root@vpn-fr.example.com:/usr/local/bin/
```

Si le VPS est ARM : `GOARCH=arm64 make build-agent`.

**Pourquoi.** L'agent est un binaire Linux. Il doit tourner **sur le VPS**, à côté de `wg0`, parce que configurer WireGuard se fait en local sur la machine (droit `CAP_NET_ADMIN`). L'API, elle, peut rester ailleurs : elle ne touche jamais `wg0` directement, elle demande à l'agent de le faire.

---

## 5. Donner un certificat TLS à l'agent

Sur la machine qui garde la clé de la CA :

```bash
NAME=vpn-fr
IP=203.0.113.10
DNS=vpn-fr.example.com

openssl req -newkey ed25519 -nodes -keyout $NAME.key -out $NAME.csr -subj "/CN=$NAME"
openssl x509 -req -in $NAME.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out $NAME.crt -days 825 \
  -extfile <(printf "subjectAltName=DNS:%s,IP:%s\nextendedKeyUsage=serverAuth" "$DNS" "$IP")

scp $NAME.crt $NAME.key root@$DNS:/etc/wg-agent/
```

`ca.crt` est le certificat de la CA. On le met aussi sur la machine de l'API, par exemple dans `/etc/wg-api/ca.crt`. `ca.key` reste hors des serveurs : elle sert seulement à signer.

La CA se crée une fois, sur une machine de confiance :

```bash
sudo mkdir -p /etc/wg-api
sudo openssl req -x509 -newkey ed25519 -nodes -days 3650 \
  -keyout ca.key -out ca.crt -subj "/CN=wg-agents-ca"
sudo chmod 600 ca.key
```

**Pourquoi.** Sans TLS, `AGENT_TOKEN` voyage en clair sur Internet. Avec TLS :

- la connexion est chiffrée ;
- l'API vérifie qu'elle parle bien à **cet** agent, parce que le certificat est signé par la CA (`AGENT_TLS_CA`) ;
- le nom dans le certificat (le SAN, `vpn-fr.example.com` ou l'IP) doit être **exactement** celui mis dans `grpc_endpoint`, sans le `:50051`. Sinon l'API refuse le certificat.

Le chemin `/etc/wg-agent/vpn-fr.crt` est un choix de rangement. Un fichier par VPS, parce que chaque serveur a sa propre identité. Le chemin peut être autre, tant que `AGENT_TLS_CERT` et `AGENT_TLS_KEY` pointent vers les bons fichiers.

---

## 6. Démarrer l'agent

`/etc/wg-agent/env` :

```ini
AGENT_LISTEN=0.0.0.0:50051
WG_INTERFACE=wg0
AGENT_BACKEND=wgctrl
AGENT_TOKEN=<le secret de l'étape 1>
AGENT_TLS_CERT=/etc/wg-agent/vpn-fr.crt
AGENT_TLS_KEY=/etc/wg-agent/vpn-fr.key
```

```bash
sudo chmod 600 /etc/wg-agent/env /etc/wg-agent/*.key
```

Service `/etc/systemd/system/wg-agent.service` :

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
AmbientCapabilities=CAP_NET_ADMIN
CapabilityBoundingSet=CAP_NET_ADMIN
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now wg-agent
journalctl -u wg-agent -n 20
```

La ligne attendue ressemble à : `agent listening on [::]:50051 ... tls=true`.

**Pourquoi chaque variable.**

| Variable | Rôle |
|---|---|
| `AGENT_LISTEN` | Port sur lequel l'agent attend l'API |
| `WG_INTERFACE=wg0` | L'interface que l'agent a le droit de modifier |
| `AGENT_BACKEND=wgctrl` | Parle au vrai WireGuard du noyau. `memory` sert seulement à développer sans interface réelle |
| `AGENT_TOKEN` | Le secret de l'étape 1 |
| `AGENT_TLS_CERT` / `AGENT_TLS_KEY` | Active TLS. Les deux ensemble, sinon l'agent refuse de démarrer |

`Requires=wg-quick@wg0` : l'agent s'arrête si `wg0` n'existe pas. Mieux vaut un échec visible au boot qu'un agent qui répond « interface absente » à chaque appel. `CAP_NET_ADMIN` est le seul droit dont il a besoin pour modifier WireGuard.

---

## 7. Autoriser l'API à lui parler

Dans l'environnement de l'API :

```ini
AGENT_TOKEN=<le même secret>
AGENT_TLS_CA=/etc/wg-api/ca.crt
AGENT_SYNC_ENABLED=true
AGENT_SYNC_INTERVAL=30s
```

Puis redémarre l'API pour qu'elle relise ces variables.

**Pourquoi.** `AGENT_TLS_CA` est le certificat public de la CA : l'API s'en sert pour accepter les certificats des agents. `AGENT_SYNC_ENABLED` lance une boucle : toutes les 30 secondes, l'API relit les appareils en base et les réapplique sur chaque `wg0`. Si un push a raté parce que l'agent était arrêté, il est rattrapé au tour suivant. La base reste la source de vérité, pas le serveur VPN.

---

## 8. Déclarer le serveur dans l'API

Connecte-toi pour obtenir un JWT, puis :

```bash
curl -s -X POST https://api.example.com/api/v1/server \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "region": "fr",
    "name": "Paris #1",
    "public_endpoint": "vpn-fr.example.com:51820",
    "wg_public_key": "<clé publique affichée à l étape 2>",
    "grpc_endpoint": "vpn-fr.example.com:50051",
    "subnet": "10.8.0.0/24",
    "max_peers": 200
  }'
```

**Pourquoi chaque champ.** L'API ne découvre pas les VPS toute seule. Cet appel est la fiche du serveur.

| Champ | À quoi il sert |
|---|---|
| `region` / `name` | Affichés dans l'app pour choisir un serveur |
| `public_endpoint` | Adresse **UDP** mise dans la config WireGuard du client. C'est là que le téléphone se connecte |
| `wg_public_key` | Clé du serveur, aussi mise dans la config du client. Doit être celle de l'étape 2, sinon le tunnel ne s'établit pas |
| `grpc_endpoint` | Adresse **TCP** de l'agent. C'est là que l'API se connecte. Doit correspondre au SAN du certificat |
| `subnet` | Le même bloc que `WG_SUBNET`. L'API y attribue `10.8.0.2`, `10.8.0.3`, etc. |
| `max_peers` | Plafond d'appareils sur ce serveur |

Aucun redémarrage de l'API après ce `POST`. Au prochain tour (ou au premier appel), elle ouvre la connexion vers `grpc_endpoint`.

---

## 9. Vérifier

```bash
curl -s https://api.example.com/api/v1/server/<id>/agent/status \
  -H "Authorization: Bearer $TOKEN"
```

La réponse attendue a `interface_up: true`. Dans les logs de l'API : `reconciler: Paris #1 (vpn-fr.example.com:50051): 0 peers`.

**Pourquoi ce test.** Il fait le vrai appel gRPC. S'il répond, le token, le TLS, le firewall et `wg0` sont bons. Ensuite, créer un appareil (`POST /api/v1/device` avec ce `server_id`) pousse un peer sur `wg0`. `agent_synced: true` veut dire que le push a réussi tout de suite.

Pour forcer une synchro sans attendre 30 secondes :

```bash
curl -s -X POST https://api.example.com/api/v1/server/<id>/agent/sync \
  -H "Authorization: Bearer $TOKEN"
```

---

## Quand ça échoue

| Ce que tu vois | Ce que ça veut dire |
|---|---|
| `503` | L'API n'atteint pas l'agent : processus arrêté, mauvais `grpc_endpoint`, ou port 50051 filtré |
| `Unauthenticated` | Les deux `AGENT_TOKEN` ne sont pas les mêmes |
| erreur `x509` | Le nom du certificat n'est pas celui de `grpc_endpoint` |
| `interface_up: false` | `wg0` n'est pas démarré : `systemctl status wg-quick@wg0` |
| `agent_synced: false` à la création d'un appareil | L'agent était injoignable à cet instant. Le tour suivant le remet |

Sur le VPS : `wg show wg0`, `ss -ltnp | grep 50051`, `journalctl -u wg-agent -n 50`.

Un deuxième pays, c'est les mêmes étapes avec un autre subnet et un autre certificat. L'API n'a pas à être redéployée.
