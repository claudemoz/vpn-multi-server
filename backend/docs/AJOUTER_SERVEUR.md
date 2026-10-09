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

**Pourquoi.** Sans TLS, `AGENT_TOKEN` voyage en clair sur Internet. Avec TLS :

- la connexion entre l'API et l'agent est chiffrée ;
- l'API vérifie qu'elle parle bien à **cet** agent et pas à un imposteur, parce que son certificat est signé par notre CA (`AGENT_TLS_CA`).

Il y a deux sortes de fichiers :

| Fichier | Rôle | Où il vit |
|---|---|---|
| `ca.key` | clé de la CA, sert **uniquement à signer** | une machine de confiance (ton poste), **jamais** sur un serveur |
| `ca.crt` | certificat public de la CA | la machine de l'API (`/etc/wg-api/ca.crt`) |
| `vpn-xx.key` | clé privée de l'agent | le VPS concerné uniquement |
| `vpn-xx.crt` | certificat de l'agent, signé par la CA | le VPS concerné uniquement |

La CA se crée **une seule fois**, pour le premier serveur. Pour chaque nouveau serveur, on ne fait que l'étape 5.2 avec la même CA : rien à changer sur l'API ni sur les autres serveurs.

### 5.1 Créer la CA (une seule fois)

Sur ta machine de confiance, dans un dossier dédié :

```bash
mkdir -p ~/wg-ca && cd ~/wg-ca
openssl req -x509 -newkey ed25519 -nodes -days 3650 \
  -keyout ca.key -out ca.crt -subj "/CN=wg-agents-ca"
chmod 600 ca.key

scp ca.crt root@<MACHINE_API>:/etc/wg-api/ca.crt
```

**Pourquoi.** La CA est l'autorité que l'API croit. Tout certificat qu'elle signe est accepté par l'API. Elle dure 10 ans (`-days 3650`). Si `ca.key` est perdue, il faut recréer une CA et re-signer tous les agents. Si elle fuit, n'importe qui peut se faire passer pour un agent : garde-la hors des serveurs et sauvegarde-la.

### 5.2 Créer le certificat d'un serveur (à chaque nouveau serveur)

Dans le dossier de la CA, adapte les trois variables :

```bash
cd ~/wg-ca
NAME=vpn-fr                  # nom du serveur, un certificat par VPS
IP=203.0.113.10              # IP publique du VPS
DNS=vpn-fr.example.com       # nom DNS du VPS

# 1. Clé privée de l'agent + demande de signature (CSR)
openssl req -newkey ed25519 -nodes -keyout $NAME.key -out $NAME.csr -subj "/CN=$NAME"

# 2. Signature par la CA
openssl x509 -req -in $NAME.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out $NAME.crt -days 825 \
  -extfile <(printf "subjectAltName=DNS:%s,IP:%s\nextendedKeyUsage=serverAuth" "$DNS" "$IP")
```

Sans nom DNS, garde seulement l'IP :

```bash
openssl x509 -req -in $NAME.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out $NAME.crt -days 825 \
  -extfile <(printf "subjectAltName=IP:%s\nextendedKeyUsage=serverAuth" "$IP")
```

**Pourquoi.**

- La **CSR** contient la clé publique de l'agent et son nom. La CA la signe : c'est ce qui rend le certificat digne de confiance pour l'API.
- Le **SAN** (`subjectAltName`) liste les noms sous lesquels l'agent peut être joint. L'API compare l'hôte de `grpc_endpoint` (sans le `:50051`) à cette liste. S'il n'y figure pas **exactement**, la connexion est refusée.
- `extendedKeyUsage=serverAuth` limite le certificat à un usage serveur TLS.
- `-CAcreateserial` crée `ca.srl`, qui donne un numéro unique à chaque certificat signé. Garde-le à côté de la CA.
- 825 jours, c'est la durée maximale acceptée par les systèmes Apple et une bonne habitude ; il faudra renouveler (voir 5.5).

### 5.3 Vérifier le certificat

```bash
openssl verify -CAfile ca.crt $NAME.crt                # doit afficher : vpn-fr.crt: OK
openssl x509 -in $NAME.crt -noout -ext subjectAltName  # doit lister le DNS et/ou l'IP
openssl x509 -in $NAME.crt -noout -enddate             # date d'expiration
```

**Pourquoi.** Une faute de frappe dans l'IP ou le DNS ne se voit qu'au moment où l'API refuse de se connecter. Vérifier ici évite de chercher l'erreur dans les logs.

### 5.4 Envoyer le certificat sur le VPS

```bash
ssh root@$IP "mkdir -p /etc/wg-agent"
scp $NAME.crt $NAME.key root@$IP:/etc/wg-agent/
ssh root@$IP "chmod 600 /etc/wg-agent/$NAME.key"
rm $NAME.csr
```

Puis, dans `/etc/wg-agent/env` sur le VPS (étape 6) :

```ini
AGENT_TLS_CERT=/etc/wg-agent/vpn-fr.crt
AGENT_TLS_KEY=/etc/wg-agent/vpn-fr.key
```

**Pourquoi.** La clé `$NAME.key` est l'identité de l'agent : elle ne doit exister que sur son VPS (`chmod 600` : lisible par root seulement). La CSR ne sert plus après la signature. Le chemin `/etc/wg-agent/` est un choix de rangement ; il peut être autre tant que `AGENT_TLS_CERT` et `AGENT_TLS_KEY` pointent vers les bons fichiers.

### 5.5 Tester depuis la machine de l'API

Une fois l'agent démarré (étape 6) :

```bash
openssl s_client -connect vpn-fr.example.com:50051 -CAfile /etc/wg-api/ca.crt \
  -verify_hostname vpn-fr.example.com </dev/null 2>/dev/null | grep "Verify return code"
# Verify return code: 0 (ok)
```

**Pourquoi.** C'est exactement la vérification que fait l'API. Si ce test passe, la connexion TLS de l'API passera aussi. Si l'endpoint est une IP, remplace `-verify_hostname` par `-verify_ip 203.0.113.10`.

### 5.6 Renouveler un certificat

Avant la date d'expiration (`openssl x509 -noout -enddate`), refais 5.2 à 5.4 avec le même `NAME`, puis sur le VPS :

```bash
systemctl restart wg-agent
```

**Pourquoi.** Un certificat expiré est refusé par l'API : l'agent devient injoignable et les nouveaux appareils ne peuvent plus se connecter à ce serveur. Les tunnels déjà établis continuent de fonctionner, car ils ne passent pas par l'agent.

### Erreurs fréquentes

- **`x509: certificate is valid for X, not Y`** : l'hôte de `grpc_endpoint` n'est pas dans le SAN. Corrige `grpc_endpoint` ou re-signe avec le bon DNS/IP.
- **`x509: certificate signed by unknown authority`** : l'API n'utilise pas la bonne `ca.crt` (`AGENT_TLS_CA`), ou le certificat a été signé par une autre CA.
- **`Algorithm ed25519 not found` sur Mac** : l'`openssl` d'Apple est LibreSSL. Utilise celui de Homebrew (`brew install openssl@3`, puis `$(brew --prefix openssl@3)/bin/openssl`) ou fais la manipulation sur une machine Linux.
- **`-extfile <(...)` ne marche pas** : cette syntaxe demande `bash` ou `zsh`, pas `sh`.

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
