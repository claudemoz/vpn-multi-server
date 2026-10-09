import { router } from 'expo-router';
import { ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button } from '@/components/ui/Button';
import { MenuRow } from '@/components/ui/MenuRow';
import { Palette, Spacing } from '@/constants/theme';
import { useAuth } from '@/contexts/AuthContext';
import { useVpn } from '@/contexts/VpnContext';

const STATUS_LABEL = {
  disconnected: 'Déconnecté',
  connecting: 'Connexion…',
  connected: 'Connecté',
  disconnecting: 'Déconnexion…',
} as const;

export default function AccountScreen() {
  const { user, logout } = useAuth();
  const { status, assignedIp, selectedServer } = useVpn();
  const initials = `${user?.first_name?.[0] ?? ''}${user?.last_name?.[0] ?? ''}`.toUpperCase() || '?';

  return (
    <SafeAreaView style={styles.safe} edges={['top', 'left', 'right']}>
      <ScrollView contentContainerStyle={styles.content}>
        <Text style={styles.title}>Compte</Text>

        <View style={styles.identity}>
          <View style={styles.avatar}>
            <Text style={styles.avatarText}>{initials}</Text>
          </View>
          <Text style={styles.name}>
            {user?.first_name} {user?.last_name}
          </Text>
          <Text style={styles.email}>{user?.email}</Text>
        </View>

        <Section title="Profil">
          <MenuRow
            icon="person-outline"
            label="Modifier le profil"
            onPress={() => router.push('/(app)/account/profile')}
            last
          />
        </Section>

        <Section title="Paramètres">
          <MenuRow
            icon="options-outline"
            label="Paramètres"
            onPress={() => router.push('/(app)/account/settings')}
            last
          />
        </Section>

        <Section title="Sécurité">
          <MenuRow
            icon="key-outline"
            label="Changer le mot de passe"
            onPress={() => router.push('/(app)/account/password')}
          />
          <MenuRow
            icon="trash-outline"
            label="Supprimer le compte"
            danger
            onPress={() => router.push('/(app)/account/delete')}
            last
          />
        </Section>

        <Section title="Connexion">
          <InfoRow label="État" value={STATUS_LABEL[status]} />
          <InfoRow label="Serveur" value={selectedServer?.name} />
          <InfoRow label="IP du tunnel" value={assignedIp} last />
        </Section>

        <Button title="Se déconnecter" variant="secondary" onPress={() => logout().catch(() => {})} />
      </ScrollView>
    </SafeAreaView>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <View style={styles.section}>
      <Text style={styles.sectionTitle}>{title}</Text>
      <View style={styles.card}>{children}</View>
    </View>
  );
}

function InfoRow({ label, value, last }: { label: string; value?: string | null; last?: boolean }) {
  return (
    <View style={[styles.infoRow, !last && styles.infoBorder]}>
      <Text style={styles.infoLabel}>{label}</Text>
      <Text style={styles.infoValue} numberOfLines={1} selectable>
        {value || '—'}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  safe: {
    flex: 1,
    backgroundColor: Palette.surface,
  },
  content: {
    padding: Spacing.four,
    gap: Spacing.four,
    paddingBottom: Spacing.five,
  },
  title: {
    fontSize: 28,
    fontWeight: '600',
    color: '#fff',
  },
  identity: {
    alignItems: 'center',
    gap: 4,
  },
  avatar: {
    width: 72,
    height: 72,
    borderRadius: 36,
    backgroundColor: Palette.blue + '26',
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: Spacing.two,
  },
  avatarText: {
    color: Palette.blue,
    fontSize: 24,
    fontWeight: '700',
  },
  name: {
    fontSize: 20,
    fontWeight: '600',
    color: '#fff',
  },
  email: {
    fontSize: 14,
    color: Palette.muted,
  },
  section: {
    gap: Spacing.two,
  },
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
  infoRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    gap: Spacing.three,
    paddingVertical: 14,
  },
  infoBorder: {
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: Palette.border,
  },
  infoLabel: {
    fontSize: 14,
    color: Palette.muted,
  },
  infoValue: {
    flexShrink: 1,
    fontSize: 14,
    fontWeight: '600',
    color: '#fff',
  },
});
