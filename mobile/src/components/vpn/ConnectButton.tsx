import { useEffect, useRef } from 'react';
import { ActivityIndicator, Animated, Easing, Pressable, StyleSheet, Text, View } from 'react-native';

import { Palette } from '@/constants/theme';
import type { ConnectionStatus } from '@/contexts/VpnContext';

type Props = {
  status: ConnectionStatus;
  onPress: () => void;
  disabled?: boolean;
};

const STATUS_LABEL: Record<ConnectionStatus, string> = {
  disconnected: 'Appuyer pour se connecter',
  connecting: 'Toucher pour annuler',
  connected: 'Appuyer pour se déconnecter',
  disconnecting: 'Déconnexion…',
};

const STATUS_COLOR: Record<ConnectionStatus, string> = {
  disconnected: Palette.blue,
  connecting: Palette.amber,
  connected: Palette.green,
  disconnecting: Palette.slate,
};

const SIZE = 124;
const HALO_COUNT = 2;

export function ConnectButton({ status, onPress, disabled }: Props) {
  const color = STATUS_COLOR[status];
  const isBusy = status === 'connecting' || status === 'disconnecting';
  const isActive = status === 'connecting' || status === 'connected';

  const halos = useRef(Array.from({ length: HALO_COUNT }, () => new Animated.Value(0))).current;
  const press = useRef(new Animated.Value(1)).current;

  useEffect(() => {
    if (!isActive) {
      halos.forEach((h) => h.setValue(0));
      return;
    }
    const duration = status === 'connecting' ? 1400 : 2600;
    const loops = halos.map((h, i) =>
      Animated.loop(
        Animated.sequence([
          Animated.delay((duration / HALO_COUNT) * i),
          Animated.timing(h, {
            toValue: 1,
            duration,
            easing: Easing.out(Easing.quad),
            useNativeDriver: true,
          }),
          Animated.timing(h, { toValue: 0, duration: 0, useNativeDriver: true }),
        ]),
      ),
    );
    loops.forEach((l) => l.start());
    return () => loops.forEach((l) => l.stop());
  }, [isActive, status, halos]);

  function animatePress(toValue: number) {
    Animated.spring(press, { toValue, useNativeDriver: true, speed: 40, bounciness: 6 }).start();
  }

  const isDisabled = disabled || status === 'disconnecting';

  return (
    <View style={styles.wrapper}>
      <View style={styles.stage}>
        {halos.map((h, i) => (
          <Animated.View
            key={i}
            pointerEvents="none"
            style={[
              styles.halo,
              {
                borderColor: color,
                opacity: h.interpolate({ inputRange: [0, 1], outputRange: [0.5, 0] }),
                transform: [{ scale: h.interpolate({ inputRange: [0, 1], outputRange: [1, 1.55] }) }],
              },
            ]}
          />
        ))}

        <View style={[styles.glow, { backgroundColor: color + '22' }]} />

        <Pressable
          onPress={onPress}
          onPressIn={() => animatePress(0.94)}
          onPressOut={() => animatePress(1)}
          disabled={isDisabled}
          accessibilityRole="button"
          accessibilityLabel={STATUS_LABEL[status]}
          accessibilityState={{ disabled: isDisabled, busy: isBusy }}
          hitSlop={12}>
          <Animated.View
            style={[
              styles.ring,
              { borderColor: color + '55', transform: [{ scale: press }] },
              isDisabled && styles.disabled,
            ]}>
            <View style={[styles.circle, { backgroundColor: color, shadowColor: color }]}>
              {isBusy ? (
                <ActivityIndicator size="large" color="#fff" />
              ) : (
                <View style={styles.powerIcon}>
                  <View style={[styles.powerArc, { borderTopColor: color }]} />
                  <View style={[styles.powerStem, { borderColor: color }]} />
                </View>
              )}
            </View>
          </Animated.View>
        </Pressable>
      </View>

      <Text style={styles.label}>{STATUS_LABEL[status]}</Text>
    </View>
  );
}

const RING = SIZE + 28;

const styles = StyleSheet.create({
  wrapper: {
    alignItems: 'center',
    gap: 14,
  },
  stage: {
    width: RING * 1.25,
    height: RING * 1.25,
    alignItems: 'center',
    justifyContent: 'center',
  },
  halo: {
    position: 'absolute',
    width: RING,
    height: RING,
    borderRadius: RING / 2,
    borderWidth: 2,
  },
  glow: {
    position: 'absolute',
    width: RING * 1.25,
    height: RING * 1.25,
    borderRadius: (RING * 1.25) / 2,
  },
  ring: {
    width: RING,
    height: RING,
    borderRadius: RING / 2,
    borderWidth: 6,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: Palette.surface,
  },
  disabled: {
    opacity: 0.5,
  },
  circle: {
    width: SIZE,
    height: SIZE,
    borderRadius: SIZE / 2,
    alignItems: 'center',
    justifyContent: 'center',
    shadowOffset: { width: 0, height: 0 },
    shadowOpacity: 0.6,
    shadowRadius: 24,
    elevation: 12,
  },
  powerIcon: {
    width: 48,
    height: 48,
    alignItems: 'center',
  },
  powerArc: {
    position: 'absolute',
    top: 8,
    width: 40,
    height: 40,
    borderRadius: 20,
    borderWidth: 4.5,
    borderColor: '#fff',
  },
  powerStem: {
    width: 13,
    height: 26,
    borderRadius: 3,
    backgroundColor: '#fff',
    borderLeftWidth: 5,
    borderRightWidth: 5,
  },
  label: {
    fontSize: 14,
    fontWeight: '500',
    color: Palette.muted,
  },
});
