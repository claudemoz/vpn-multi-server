import { useEffect, useState } from 'react';
import { RefreshControl, ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { StatusBar } from 'expo-status-bar';

import { ConnectButton } from '@/components/vpn/ConnectButton';
import { LocationLine } from '@/components/vpn/LocationLine';
import { ServerPicker } from '@/components/vpn/ServerPicker';
import { useVpn } from '@/contexts/VpnContext';
import { Palette, Spacing } from '@/constants/theme';

function formatDuration(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  return [h, m, s].map((n) => String(n).padStart(2, '0')).join(':');
}

function useElapsed(since: number | null): string | null {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (since === null) return;
    setNow(Date.now());
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [since]);

  return since === null ? null : formatDuration(now - since);
}

export default function HomeScreen() {
  const {
    servers,
    latencies,
    selectedServer,
    status,
    connectedAt,
    isLoadingServers,
    tunnelAvailable,
    error,
    autoSelect,
    selectServer,
    selectAuto,
    refreshServers,
    connect,
    disconnect,
  } = useVpn();

  const isConnected = status === 'connected';
  const isIdle = status === 'disconnected';
  const duration = useElapsed(isConnected ? connectedAt : null);
  const [refreshing, setRefreshing] = useState(false);

  async function handleRefresh() {
    setRefreshing(true);
    try {
      await refreshServers();
    } finally {
      setRefreshing(false);
    }
  }

  function handleToggle() {
    if (status === 'disconnecting') return;
    // Pressing while connecting cancels the attempt.
    const action = isIdle ? connect : disconnect;
    action().catch(() => {});
  }

  return (
    <SafeAreaView style={styles.safe} edges={['top', 'left', 'right']}>
      <StatusBar style="light" />
      <ScrollView
        contentContainerStyle={styles.content}
        showsVerticalScrollIndicator={false}
        refreshControl={
          <RefreshControl refreshing={refreshing} onRefresh={handleRefresh} tintColor={Palette.muted} />
        }>
        <View style={styles.hero}>
          <ConnectButton status={status} onPress={handleToggle} disabled={isIdle && !selectedServer} />

          <LocationLine
            status={status}
            detail={
              isConnected
                ? [duration ?? '00:00:00', selectedServer?.name].filter(Boolean).join('  ·  ')
                : null
            }
          />
        </View>

        {error ? <Text style={styles.error}>{error}</Text> : null}
        {!tunnelAvailable ? (
          <Text style={styles.warn}>VPN natif indisponible dans Expo Go : lancez un build natif.</Text>
        ) : null}

        <View style={styles.sectionHeader}>
          <Text style={styles.sectionTitle}>Serveurs</Text>
          {!isIdle ? <Text style={styles.lockText}>Verrouillé</Text> : null}
        </View>
        <ServerPicker
          servers={servers}
          latencies={latencies}
          selected={selectedServer}
          auto={autoSelect}
          onSelect={selectServer}
          onSelectAuto={selectAuto}
          disabled={!isIdle}
          loading={isLoadingServers && servers.length === 0}
        />

      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: {
    flex: 1,
    backgroundColor: Palette.surface,
  },
  content: {
    paddingHorizontal: Spacing.four,
    paddingBottom: Spacing.four,
  },
  hero: {
    alignItems: 'center',
    paddingTop: Spacing.five,
    paddingBottom: Spacing.two,
  },
  error: {
    marginTop: Spacing.three,
    padding: 12,
    borderRadius: 12,
    overflow: 'hidden',
    backgroundColor: Palette.red + '1A',
    color: '#FCA5A5',
    fontSize: 14,
    textAlign: 'center',
  },
  warn: {
    marginTop: Spacing.three,
    padding: 12,
    borderRadius: 12,
    overflow: 'hidden',
    backgroundColor: Palette.amber + '1A',
    color: '#FCD34D',
    fontSize: 13,
    textAlign: 'center',
  },
  sectionHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginTop: Spacing.four,
    marginBottom: Spacing.two,
  },
  sectionTitle: {
    fontSize: 12,
    fontWeight: '700',
    letterSpacing: 0.8,
    textTransform: 'uppercase',
    color: Palette.muted,
    paddingHorizontal: Spacing.one,
  },
  lockText: {
    fontSize: 12,
    fontWeight: '500',
    color: Palette.muted,
  },
  pullHint: {
    marginTop: Spacing.two,
    fontSize: 11,
    color: Palette.slate,
    textAlign: 'center',
  },
});
