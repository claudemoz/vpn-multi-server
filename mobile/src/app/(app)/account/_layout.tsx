import { Stack } from 'expo-router';

import { Palette } from '@/constants/theme';

export default function AccountLayout() {
  return (
    <Stack
      screenOptions={{
        headerTintColor: '#fff',
        headerStyle: { backgroundColor: Palette.surface },
        headerShadowVisible: false,
        headerBackTitle: 'Retour',
        headerTitleStyle: { fontWeight: '600' },
        contentStyle: { backgroundColor: Palette.surface },
      }}>
      <Stack.Screen name="index" options={{ headerShown: false }} />
      <Stack.Screen name="profile" options={{ title: 'Profil' }} />
      <Stack.Screen name="password" options={{ title: 'Mot de passe' }} />
      <Stack.Screen name="settings" options={{ title: 'Paramètres' }} />
      <Stack.Screen name="delete" options={{ title: 'Supprimer le compte' }} />
    </Stack>
  );
}
