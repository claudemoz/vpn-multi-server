import type { Ref } from 'react';
import { StyleSheet, Text, TextInput, View, type TextInputProps } from 'react-native';

import { Palette, Spacing } from '@/constants/theme';

type Props = TextInputProps & {
  label: string;
  error?: string;
  ref?: Ref<TextInput>;
};

export function Input({ label, error, style, ref, ...rest }: Props) {
  return (
    <View style={styles.wrapper}>
      <Text style={styles.label}>{label}</Text>
      <TextInput
        ref={ref}
        style={[styles.input, error && styles.inputError, style]}
        placeholderTextColor={Palette.muted}
        autoCapitalize="none"
        {...rest}
      />
      {error ? <Text style={styles.error}>{error}</Text> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrapper: {
    gap: Spacing.one,
  },
  label: {
    fontSize: 14,
    fontWeight: '500',
    color: Palette.muted,
  },
  input: {
    backgroundColor: Palette.cardRaised,
    borderRadius: 12,
    paddingHorizontal: Spacing.three,
    paddingVertical: Spacing.two + 4,
    fontSize: 16,
    color: '#fff',
    borderWidth: 1,
    borderColor: Palette.border,
  },
  inputError: {
    borderColor: Palette.red,
  },
  error: {
    fontSize: 13,
    color: Palette.red,
  },
});
