import { StyleSheet, Switch, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Palette, Spacing } from '@/constants/theme';
import { useVpn } from '@/contexts/VpnContext';

export default function SettingsScreen() {
  const { autoSelect, selectedServer, servers, status, selectAuto, selectServer } = useVpn();
  const canAuto = servers.filter((s) => s.is_active).length > 1;
  const locked = status !== 'disconnected';

  function toggleAuto(value: boolean) {
    if (locked) return;
    if (value) selectAuto();
    else if (selectedServer) selectServer(selectedServer);
  }

  return (
    <SafeAreaView style={styles.safe} edges={['bottom']}>
      <View style={styles.content}>
        <Text style={styles.sectionTitle}>Connexion</Text>
        <View style={styles.card}>
          <View style={styles.row}>
            <View style={styles.copy}>
              <Text style={styles.label}>Serveur automatique</Text>
              <Text style={styles.hint}>
                {canAuto
                  ? 'Choisit le serveur le moins chargé à la connexion.'
                  : 'Disponible dès qu’un second serveur est actif.'}
              </Text>
            </View>
            <Switch
              value={autoSelect && canAuto}
              onValueChange={toggleAuto}
              disabled={!canAuto || locked}
              trackColor={{ false: Palette.border, true: Palette.blue + '99' }}
              thumbColor={autoSelect && canAuto ? Palette.blue : Palette.muted}
            />
          </View>
        </View>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Palette.surface },
  content: { padding: Spacing.four, gap: Spacing.two },
  sectionTitle: {
    fontSize: 12,
    fontWeight: '600',
    letterSpacing: 0.6,
    textTransform: 'uppercase',
    color: Palette.muted,
    paddingHorizontal: Spacing.one,
  },
  card: {
    backgroundColor: Palette.card,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: Palette.border,
    paddingHorizontal: Spacing.three,
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.three,
    paddingVertical: 14,
  },
  copy: { flex: 1, gap: 4 },
  label: { fontSize: 16, color: '#fff' },
  hint: { fontSize: 13, color: Palette.muted, lineHeight: 18 },
});
