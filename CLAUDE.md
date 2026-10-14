# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Monorepo d'une plateforme VPN WireGuard : un control plane Go + un agent par serveur VPN (`backend/`) et une app cliente Expo/React Native (`mobile/`, qui a son propre `CLAUDE.md` → `AGENTS.md`). La documentation est majoritairement en français.

## Backend (`backend/`, module Go `backend`)

Control plane HTTP (Gin + PostgreSQL/GORM) et agent gRPC par serveur VPN. Commandes, règles et architecture : voir `backend/CLAUDE.md`.

## Mobile (`mobile/`)

Expo SDK 54, expo-router (routes par fichiers dans `src/app/`, groupes `(auth)` et `(app)`), TypeScript, alias `@/` → `src/`. **Lire la doc Expo versionnée (https://docs.expo.dev/versions/v54.0.0/) avant d'écrire du code**, comme l'exige `mobile/AGENTS.md`.

```bash
npm install          # applique aussi patches/ via patch-package (postinstall)
npm run ios          # expo run:ios --device
npm run android
npm run lint         # expo lint (aucun framework de test configuré)
```

Le tunnel utilise `react-native-wireguard-vpn` (Network Extension sur iOS, VpnService sur Android) : **Expo Go ne fonctionne pas**, il faut un development build. La librairie est patchée pour RN 0.81 (`patches/`) ; iOS demande des étapes manuelles dans Xcode, voir `docs/VPN_NATIVE_SETUP.md`.

Flux de l'app : `AuthContext` (JWT stocké dans expo-secure-store) et `VpnContext` pilotent l'UI ; `services/api.ts` appelle `/api/v1` (hôte détecté en dev via le `hostUri` de Metro, en prod via `EXPO_PUBLIC_API_URL`) ; la config wg-quick du device, obtenue par `GET /devices/:id/config`, est parsée par `services/wg-config-parser.ts` puis transmise à `services/vpn-tunnel.ts`.
