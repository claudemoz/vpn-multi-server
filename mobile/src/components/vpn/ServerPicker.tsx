import Ionicons from '@expo/vector-icons/Ionicons';
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';

import { Palette, Spacing } from '@/constants/theme';
import { countryName } from '@/services/geo';
import { loadOf, pickBestServer, type Latencies } from '@/services/server-select';
import type { Server } from '@/types/api';

type Props = {
  servers: Server[];
  /** Measured round trip per server id (ms); empty until measured. */
  latencies?: Latencies;
  /** Server that will be used; chosen by the app when `auto` is on. */
  selected: Server | null;
  auto: boolean;
  onSelect: (server: Server) => void;
  onSelectAuto: () => void;
  disabled?: boolean;
  loading?: boolean;
};

export function regionFlag(region: string): string {
  const code = region.trim().slice(0, 2).toUpperCase();
  if (!/^[A-Z]{2}$/.test(code)) return '';
  return String.fromCodePoint(...[...code].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
}

function signal(load: number): { bars: number; color: string; label: string } {
  if (load >= 0.85) return { bars: 1, color: Palette.red, label: 'Saturé' };
  if (load >= 0.6) return { bars: 2, color: Palette.amber, label: 'Chargé' };
  return { bars: 3, color: Palette.green, label: 'Rapide' };
}

export function ServerPicker({
  servers,
  latencies = {},
  selected,
  auto,
  onSelect,
  onSelectAuto,
  disabled,
  loading,
}: Props) {
  if (loading) {
    return (
      <View style={[styles.card, styles.placeholder]}>
        <ActivityIndicator color={Palette.blue} />
        <Text style={styles.placeholderText}>Chargement des serveurs…</Text>
      </View>
    );
  }

  if (servers.length === 0) {
    return (
      <View style={[styles.card, styles.placeholder]}>
        <Text style={styles.placeholderText}>Aucun serveur disponible</Text>
      </View>
    );
  }

  const active = servers.filter((s) => s.is_active);
  const showAuto = active.length > 1;
  const autoOn = showAuto && auto;
  const recommendedId = showAuto ? (pickBestServer(active, null, latencies)?.id ?? null) : null;

  return (
    <View style={styles.card}>
      {showAuto ? (
        <Pressable
          disabled={disabled}
          onPress={onSelectAuto}
          accessibilityRole="radio"
          accessibilityState={{ selected: autoOn, disabled }}
          accessibilityLabel="Sélection automatique du serveur"
          style={({ pressed }) => [
            styles.row,
            autoOn && styles.rowSelected,
            pressed && styles.rowPressed,
            disabled && !autoOn && styles.rowDisabled,
          ]}>
          <View style={[styles.flagBox, styles.autoBox]}>
            <Ionicons name="globe" size={20} color={Palette.blue} />
          </View>
          <View style={styles.body}>
            <Text style={styles.name}>Automatique</Text>
            {autoOn && selected ? (
              <View style={styles.autoPick}>
                <Ionicons name="server" size={11} color={Palette.blue} />
                <Text style={styles.country} numberOfLines={1}>
                  {selected.name}  ·  {countryName(selected.region)}
                </Text>
              </View>
            ) : (
              <Text style={styles.country} numberOfLines={1}>
                Le serveur le plus rapide pour vous
              </Text>
            )}
          </View>
          <Radio on={autoOn} />
        </Pressable>
      ) : null}

      {servers.map((server, index) => {
        const isSelected = !autoOn && selected?.id === server.id;
        const unavailable = !server.is_active;
        const rowDisabled = disabled || unavailable;
        const load = loadOf(server);
        const sig = signal(load);
        const latency = latencies[server.id];

        return (
          <Pressable
            key={server.id}
            disabled={rowDisabled}
            onPress={() => onSelect(server)}
            accessibilityRole="radio"
            accessibilityState={{ selected: isSelected, disabled: rowDisabled }}
            accessibilityLabel={`${server.name}, ${countryName(server.region)}, ${unavailable ? 'indisponible' : sig.label}`}
            style={({ pressed }) => [
              styles.row,
              (showAuto || index > 0) && styles.rowBorder,
              isSelected && styles.rowSelected,
              pressed && styles.rowPressed,
              rowDisabled && !isSelected && styles.rowDisabled,
            ]}>
            <View style={styles.flagBox}>
              {regionFlag(server.region) ? (
                <Text style={styles.flag}>{regionFlag(server.region)}</Text>
              ) : (
                <Ionicons name="globe-outline" size={20} color={Palette.muted} />
              )}
            </View>

            <View style={styles.body}>
              <View style={styles.nameLine}>
                <Text style={styles.name} numberOfLines={1}>
                  {server.name}
                </Text>
                {server.id === recommendedId ? (
                  <View style={styles.tag}>
                    <Text style={styles.tagText}>Recommandé</Text>
                  </View>
                ) : null}
              </View>
              <Text style={styles.country} numberOfLines={1}>
                {countryName(server.region)}
                {unavailable ? '  ·  Maintenance' : `  ·  ${Math.round(load * 100)} % de charge`}
                {!unavailable && latency !== undefined ? `  ·  ${latency} ms` : ''}
              </Text>
            </View>

            {unavailable ? null : <SignalBars bars={sig.bars} color={sig.color} />}

            <Radio on={isSelected} />
          </Pressable>
        );
      })}
    </View>
  );
}

function Radio({ on }: { on: boolean }) {
  return (
    <Ionicons
      name={on ? 'checkmark-circle' : 'ellipse-outline'}
      size={24}
      color={on ? Palette.blue : Palette.border}
    />
  );
}

function SignalBars({ bars, color }: { bars: number; color: string }) {
  return (
    <View style={styles.bars}>
      {[6, 10, 14].map((height, i) => (
        <View
          key={height}
          style={[styles.bar, { height, backgroundColor: i < bars ? color : Palette.border }]}
        />
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  card: {
    backgroundColor: Palette.card,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: Palette.border,
    overflow: 'hidden',
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.three,
    paddingVertical: 12,
    paddingHorizontal: 14,
  },
  rowBorder: {
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: Palette.border,
  },
  rowSelected: {
    backgroundColor: Palette.blue + '14',
  },
  rowPressed: {
    backgroundColor: Palette.cardRaised,
  },
  rowDisabled: {
    opacity: 0.45,
  },
  flagBox: {
    width: 38,
    height: 38,
    borderRadius: 19,
    backgroundColor: Palette.cardRaised,
    alignItems: 'center',
    justifyContent: 'center',
  },
  flag: {
    fontSize: 20,
  },
  autoPick: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
  },
  autoBox: {
    backgroundColor: Palette.blue + '26',
  },
  body: {
    flex: 1,
    gap: 2,
  },
  nameLine: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.two,
  },
  name: {
    flexShrink: 1,
    fontSize: 15,
    fontWeight: '600',
    color: '#fff',
  },
  tag: {
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 6,
    backgroundColor: Palette.green + '1F',
  },
  tagText: {
    fontSize: 10,
    fontWeight: '700',
    letterSpacing: 0.3,
    color: Palette.green,
  },
  country: {
    fontSize: 13,
    color: Palette.muted,
  },
  bars: {
    flexDirection: 'row',
    alignItems: 'flex-end',
    gap: 2,
    height: 14,
  },
  bar: {
    width: 4,
    borderRadius: 1,
  },
  placeholder: {
    alignItems: 'center',
    gap: Spacing.two,
    padding: Spacing.four,
  },
  placeholderText: {
    color: Palette.muted,
    fontSize: 14,
  },
});
