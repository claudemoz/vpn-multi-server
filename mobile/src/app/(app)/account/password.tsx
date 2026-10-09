import { router } from 'expo-router';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { KeyboardAvoidingView, Platform, ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button } from '@/components/ui/Button';
import { FormInput } from '@/components/ui/FormInput';
import { Palette, Spacing } from '@/constants/theme';
import { useAuth } from '@/contexts/AuthContext';
import { ApiClientError } from '@/services/api';
import { confirmPasswordRules, loginPasswordRules, passwordRules } from '@/services/auth-rules';

type Values = {
  currentPassword: string;
  password: string;
  confirmPassword: string;
};

export default function PasswordScreen() {
  const { changePassword } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const { control, handleSubmit, formState, watch, trigger } = useForm<Values>({
    defaultValues: { currentPassword: '', password: '', confirmPassword: '' },
    mode: 'onSubmit',
    reValidateMode: 'onChange',
  });
  const password = watch('password');

  const onSubmit = handleSubmit(async ({ currentPassword, password }) => {
    setError(null);
    try {
      await changePassword({ current_password: currentPassword, password });
      router.back();
    } catch (e) {
      setError(
        e instanceof ApiClientError ? e.message : 'Impossible de changer le mot de passe.',
      );
    }
  });

  return (
    <SafeAreaView style={styles.safe} edges={['bottom']}>
      <KeyboardAvoidingView style={styles.flex} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
          <View style={styles.form}>
            <FormInput
              control={control}
              name="currentPassword"
              rules={loginPasswordRules}
              label="Mot de passe actuel"
              secureTextEntry
              autoComplete="password"
              placeholder="Saisissez votre mot de passe actuel"
            />
            <FormInput
              control={control}
              name="password"
              rules={passwordRules}
              label="Nouveau mot de passe"
              secureTextEntry
              autoComplete="new-password"
              placeholder="Saisissez un mot de passe (8 caractères min.)"
              onChangeText={() => {
                if (formState.dirtyFields.confirmPassword) trigger('confirmPassword');
              }}
            />
            <FormInput
              control={control}
              name="confirmPassword"
              rules={confirmPasswordRules(password)}
              label="Confirmer le mot de passe"
              secureTextEntry
              autoComplete="new-password"
              placeholder="Retapez le même mot de passe"
            />
            {error ? <Text style={styles.error}>{error}</Text> : null}
            <Button title="Mettre à jour" onPress={onSubmit} loading={formState.isSubmitting} style={styles.cta} />
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Palette.surface },
  flex: { flex: 1 },
  content: { padding: Spacing.four },
  form: { gap: Spacing.three },
  cta: { marginTop: Spacing.two, borderRadius: 12, shadowOpacity: 0, elevation: 0 },
  error: { color: '#FCA5A5', fontSize: 14 },
});
