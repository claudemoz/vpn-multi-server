# Tunnel VPN natif — configuration

L'app utilise [`react-native-wireguard-vpn`](https://github.com/usama7365/react-native-wireguard-vpn) pour activer le tunnel via :

> **Patch iOS (RN 0.81 / Expo 54)** : React Native a retiré `hasListeners` de `RCTEventEmitter`.
> Le fichier `patches/react-native-wireguard-vpn+1.0.22.patch` est appliqué au `npm install` via `patch-package`.


- **iOS** : Network Extension (Packet Tunnel Provider)
- **Android** : `VpnService` + consentement système

**Expo Go ne fonctionne pas.** Il faut un **development build** (`expo run:ios` / `expo run:android`).

---

## 1. Installation

```bash
cd mobile
npm install
npx expo prebuild --clean   # SDK 54 — compatible Xcode 16.3 (Swift 6.1)
```

---

## 2. Android

Le plugin Expo ajoute automatiquement les permissions VPN dans `AndroidManifest.xml`.

```bash
npm run android
```

Au premier lancement, Android affiche la **demande de consentement VPN** — acceptez-la.

---

## 3. iOS (étapes manuelles Xcode)

### Prérequis

- Compte **Apple Developer payant** ($99/an) — les Network Extensions ne fonctionnent pas avec un compte gratuit
- **Appareil physique** (le simulateur ne supporte pas le VPN)

### A. Prebuild

```bash
npm run ios
# ou
npx expo run:ios --device
```

### B. Créer l'extension Packet Tunnel

1. Ouvrir `ios/WGVPN.xcworkspace` dans Xcode
2. **File → New → Target → Network Extension → Packet Tunnel Provider**
3. Product Name : `WireGuardTunnel`
4. **Bundle Identifier** : `com.wgvpn.platform.WireGuardTunnel`  
   (généré automatiquement par Xcode = Organization Identifier + Product Name ;
   la librairie est patchée pour utiliser cet ID — voir `patches/react-native-wireguard-vpn+1.0.22.patch`,
   constante `kTunnelProviderBundleIdentifier` dans `ios/WireGuardVpn.m`)

> L'ID par défaut de la librairie, `com.wireguardvpn.tunnel`, est déjà enregistré par une autre équipe
> Apple et ne peut pas être utilisé (« cannot be registered to your development team »).

**Important** : la librairie ne fournit que le pont React Native, pas le code de l'extension.
La target créée est un template vide : il faut y implémenter `PacketTunnelProvider` avec
[WireGuardKit](https://git.zx2c4.com/wireguard-apple) (Swift Package, produit `WireGuardKit`)
en lisant `protocolConfiguration.serverAddress` et `providerConfiguration`
(`privateKey`, `publicKey`, `serverPort`, `allowedIPs`, `dns`, `mtu`).

`npx expo prebuild --clean` régénère `ios/` et **supprime cette target** : ne pas relancer
`prebuild --clean` une fois la target créée (ou la recréer ensuite).

### C. Capabilities

Sur la **cible principale** (app) :

- Signing & Capabilities → **+ Capability** → **Network Extensions**
- Cocher **Packet Tunnel**

Répéter sur la cible **WireGuardTunnel** si nécessaire.

### D. App IDs (Developer Portal)

Pour `com.wgvpn.platform` et `com.wgvpn.platform.WireGuardTunnel` :

- Activer **Network Extensions** → **Packet Tunnel**
- Regénérer les provisioning profiles

### E. Build & run

```bash
npx expo run:ios --device
```

---

## 4. Flux dans l'app

1. Login → sélection serveur → **Se connecter**
2. L'API crée un `device` et renvoie la config wg-quick + clé privée
3. `vpn-tunnel.ts` parse la config et appelle `WireGuardVpnModule.connect()`
4. iOS/Android établit le tunnel WireGuard
5. **Déconnecter** appelle `WireGuardVpnModule.disconnect()`

---

## 5. Dépannage

| Problème | Solution |
|----------|----------|
| Module non lié | `cd ios && pod install`, rebuild complet |
| iOS Code=10 permission denied | Compte dev payant + entitlements Network Extension |
| Android Bad address | Vérifier `Address = x.x.x.x/32` dans la config |
| Tunnel indisponible dans l'app | Pas Expo Go → `npm run ios` |

---

## 6. Fichiers concernés

| Fichier | Rôle |
|---------|------|
| `src/services/vpn-tunnel.ts` | Wrapper natif initialize/connect/disconnect |
| `src/services/wg-config-parser.ts` | `.conf` → `WireGuardConfig` |
| `src/contexts/VpnContext.tsx` | Orchestration API + tunnel |
| `app.json` | Plugin + entitlements iOS + permissions Android |
