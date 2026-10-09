import { Redirect, Tabs } from 'expo-router';

import { TabIcon } from '@/components/ui/TabIcon';
import { Palette } from '@/constants/theme';
import { useAuth } from '@/contexts/AuthContext';
import { VpnProvider } from '@/contexts/VpnContext';

export default function AppLayout() {
  const { token } = useAuth();

  if (!token) {
    return <Redirect href="/(auth)/login" />;
  }

  return (
    <VpnProvider>
      <Tabs
        screenOptions={{
          headerShown: false,
          tabBarActiveTintColor: Palette.blue,
          tabBarInactiveTintColor: Palette.muted,
          tabBarStyle: {
            backgroundColor: Palette.card,
            borderTopColor: Palette.border,
          },
          tabBarLabelStyle: { fontSize: 11, fontWeight: '600' },
          sceneStyle: { backgroundColor: Palette.surface },
        }}>
        <Tabs.Screen
          name="index"
          options={{
            title: 'Accueil',
            tabBarIcon: ({ color, focused }) => <TabIcon name="home" color={color} focused={focused} />,
          }}
        />
        <Tabs.Screen
          name="account"
          options={{
            title: 'Compte',
            headerShown: false,
            tabBarIcon: ({ color, focused }) => (
              <TabIcon name="account" color={color} focused={focused} />
            ),
          }}
        />
      </Tabs>
    </VpnProvider>
  );
}
