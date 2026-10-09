import {
  ActivityIndicator,
  Platform,
  Pressable,
  StyleSheet,
  Text,
  type PressableProps,
} from 'react-native';

import { Palette, Spacing } from '@/constants/theme';

type Props = PressableProps & {
  title: string;
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
  loading?: boolean;
};

export function Button({
  title,
  variant = 'primary',
  loading,
  disabled,
  style,
  ...rest
}: Props) {
  const isPrimary = variant === 'primary';
  const isGhost = variant === 'ghost';
  const isDanger = variant === 'danger';

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled: !!(disabled || loading), busy: !!loading }}
      style={({ pressed }) => [
        styles.base,
        isPrimary && styles.primary,
        variant === 'secondary' && styles.secondary,
        isGhost && styles.ghost,
        isDanger && styles.danger,
        (disabled || loading) && styles.disabled,
        pressed && !disabled && !loading && styles.pressed,
        Platform.OS === 'web' && styles.web,
        typeof style === 'function' ? style({ pressed, hovered: false }) : style,
      ]}
      disabled={disabled || loading}
      {...rest}>
      {loading ? (
        <ActivityIndicator color={isDanger ? Palette.red : isPrimary ? '#fff' : Palette.blue} />
      ) : (
        <Text style={[styles.label, isGhost && styles.labelGhost, isDanger && styles.labelDanger]}>{title}</Text>
      )}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    paddingVertical: Spacing.three,
    paddingHorizontal: Spacing.four,
    borderRadius: 16,
    alignItems: 'center',
    justifyContent: 'center',
    minHeight: 54,
  },
  primary: {
    backgroundColor: Palette.blue,
    shadowColor: Palette.blue,
    shadowOpacity: 0.45,
    shadowRadius: 16,
    shadowOffset: { width: 0, height: 6 },
    elevation: 6,
  },
  secondary: {
    backgroundColor: Palette.card,
    borderWidth: 1,
    borderColor: Palette.border,
  },
  ghost: {
    backgroundColor: 'transparent',
  },
  danger: {
    backgroundColor: 'transparent',
    borderWidth: 1,
    borderColor: Palette.red,
  },
  disabled: {
    opacity: 0.5,
  },
  pressed: {
    opacity: 0.88,
    transform: [{ scale: 0.98 }],
  },
  web: {
    cursor: 'pointer',
  } as object,
  label: {
    fontSize: 16,
    fontWeight: '700',
    color: '#fff',
  },
  labelGhost: {
    color: Palette.blue,
  },
  labelDanger: {
    color: Palette.red,
  },
});
