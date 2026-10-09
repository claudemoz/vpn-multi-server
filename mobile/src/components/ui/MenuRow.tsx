import Ionicons from '@expo/vector-icons/Ionicons';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { Palette, Spacing } from '@/constants/theme';

type Props = {
  icon: keyof typeof Ionicons.glyphMap;
  label: string;
  detail?: string;
  last?: boolean;
  danger?: boolean;
  onPress: () => void;
};

export function MenuRow({ icon, label, detail, last, danger, onPress }: Props) {
  const color = danger ? Palette.red : Palette.muted;
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="button"
      style={({ pressed }) => [styles.row, !last && styles.border, pressed && styles.pressed]}>
      <Ionicons name={icon} size={18} color={color} />
      <Text style={[styles.label, danger && styles.danger]}>{label}</Text>
      {detail ? (
        <Text style={styles.detail} numberOfLines={1}>
          {detail}
        </Text>
      ) : null}
      <Ionicons name="chevron-forward" size={16} color={Palette.slate} />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.two + 4,
    paddingVertical: 14,
  },
  border: {
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: Palette.border,
  },
  pressed: {
    opacity: 0.7,
  },
  label: {
    flex: 1,
    fontSize: 16,
    color: '#fff',
  },
  danger: {
    color: Palette.red,
  },
  detail: {
    maxWidth: 120,
    fontSize: 13,
    color: Palette.muted,
  },
});
