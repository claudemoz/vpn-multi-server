import Ionicons from '@expo/vector-icons/Ionicons';
import type { ComponentProps } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { regionFlag } from '@/components/vpn/ServerPicker';
import { Palette, Spacing } from '@/constants/theme';
import type { ConnectionStatus } from '@/contexts/VpnContext';
import { usePublicLocation } from '@/hooks/usePublicLocation';

type IconName = ComponentProps<typeof Ionicons>['name'];

type Props = {
  status: ConnectionStatus;
  detail?: string | null;
};

export function LocationLine({ status, detail }: Props) {
  const ip = usePublicLocation(status);
  const isBusy = status === 'connecting' || status === 'disconnecting';
  const loc = isBusy ? null : ip.current;
  const place = loc && ip.located ? [loc.city, loc.country].filter(Boolean).join(', ') : null;
  const flag = loc?.countryCode ? regionFlag(loc.countryCode) : '';
  const leaking = status === 'connected' && ip.isLeaking;

  let tone: string = Palette.red;
  let icon: IconName = 'shield-outline';
  let label = 'Non sécurisé';

  if (isBusy) {
    tone = Palette.amber;
    icon = 'shield-half';
    label = status === 'connecting' ? 'Connexion…' : 'Déconnexion…';
  } else if (leaking) {
    tone = Palette.amber;
    icon = 'warning-outline';
    label = 'Fuite détectée';
  } else if (status === 'connected') {
    tone = Palette.green;
    icon = 'shield-checkmark';
    label = 'Sécurisé';
  }

  return (
    <View style={styles.wrap}>
      <View style={styles.status}>
        <Ionicons name={icon} size={16} color={tone} />
        <Text style={[styles.statusText, { color: tone }]}>{label}</Text>
      </View>

      {detail ? <Text style={styles.detail}>{detail}</Text> : null}

      {isBusy || (!place && !ip.failed) ? (
        <View style={styles.bone} />
      ) : place ? (
        <Text style={styles.place} numberOfLines={1}>
          {place}
          {flag ? `  ${flag}` : ''}
        </Text>
      ) : (
        <Pressable onPress={() => ip.refresh()} hitSlop={10} accessibilityRole="button">
          <Text style={styles.retry}>Localisation indisponible · Réessayer</Text>
        </Pressable>
      )}

      {leaking ? (
        <Text style={styles.leak} numberOfLines={2}>
          {ip.leaksIpv6
            ? 'Votre IPv6 contourne le VPN. Reconnectez-vous.'
            : 'Votre adresse IP n’a pas changé. Reconnectez-vous.'}
        </Text>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    alignItems: 'center',
    gap: 8,
    marginTop: Spacing.three,
    minHeight: 68,
  },
  status: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  statusText: {
    fontSize: 15,
    fontWeight: '600',
  },
  detail: {
    fontSize: 14,
    fontWeight: '500',
    color: '#fff',
    fontVariant: ['tabular-nums'],
  },
  place: {
    fontSize: 14,
    color: Palette.muted,
  },
  leak: {
    maxWidth: 280,
    fontSize: 13,
    lineHeight: 18,
    color: Palette.amber,
    textAlign: 'center',
  },
  retry: {
    fontSize: 13,
    color: Palette.blue,
  },
  bone: {
    width: 140,
    height: 12,
    borderRadius: 6,
    backgroundColor: Palette.cardRaised,
  },
});
