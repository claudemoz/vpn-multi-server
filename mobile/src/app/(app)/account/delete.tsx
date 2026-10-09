import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { Alert, KeyboardAvoidingView, Platform, ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button } from '@/components/ui/Button';
import { FormInput } from '@/components/ui/FormInput';
import { Palette, Spacing } from '@/constants/theme';
import { useAuth } from '@/contexts/AuthContext';
import { ApiClientError } from '@/services/api';
import { loginPasswordRules } from '@/services/auth-rules';

type Values = { password: string };

export default function DeleteAccountScreen() {
  const { deleteAccount } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const { control, handleSubmit, getValues, formState } = useForm<Values>({
    defaultValues: { password: '' },
    mode: 'onSubmit',
  });

  const submit = handleSubmit(async ({ password }) => {
    setError(null);
    try {
      await deleteAccount(password);
    } catch (e) {
      setError(e instanceof ApiClientError ? e.message : 'Impossible de supprimer le compte.');
    }
  });

  function confirm() {
    const password = getValues('password');
    if (!password) {
      submit();
      return;
    }
    Alert.alert(
      'Supprimer le compte ?',
      'Cette action est définitive. Vos appareils et sessions VPN seront retirés.',
      [
        { text: 'Annuler', style: 'cancel' },
        { text: 'Supprimer', style: 'destructive', onPress: () => submit() },
      ],
    );
  }

  return (
    <SafeAreaView style={styles.safe} edges={['bottom']}>
      <KeyboardAvoidingView style={styles.flex} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
          <Text style={styles.lead}>
            La suppression est définitive. Saisissez votre mot de passe pour confirmer.
          </Text>
          <View style={styles.form}>
            <FormInput
              control={control}
              name="password"
              rules={loginPasswordRules}
              label="Mot de passe"
              secureTextEntry
              autoComplete="password"
              placeholder="Saisissez votre mot de passe"
            />
            {error ? <Text style={styles.error}>{error}</Text> : null}
            <Button
              title="Supprimer mon compte"
              variant="danger"
              onPress={confirm}
              loading={formState.isSubmitting}
              style={styles.cta}
            />
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Palette.surface },
  flex: { flex: 1 },
  content: { padding: Spacing.four, gap: Spacing.four },
  lead: { fontSize: 15, lineHeight: 22, color: Palette.muted },
  form: { gap: Spacing.three },
  cta: { marginTop: Spacing.two, borderRadius: 12 },
  error: { color: '#FCA5A5', fontSize: 14 },
});
