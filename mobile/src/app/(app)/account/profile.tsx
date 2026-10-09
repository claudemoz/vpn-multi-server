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
import { emailRules, nameRules } from '@/services/auth-rules';

type Values = {
  firstName: string;
  lastName: string;
  email: string;
};

export default function ProfileScreen() {
  const { user, updateProfile } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const { control, handleSubmit, formState } = useForm<Values>({
    defaultValues: {
      firstName: user?.first_name ?? '',
      lastName: user?.last_name ?? '',
      email: user?.email ?? '',
    },
    mode: 'onSubmit',
    reValidateMode: 'onChange',
  });

  const onSubmit = handleSubmit(async ({ firstName, lastName, email }) => {
    setError(null);
    try {
      await updateProfile({ first_name: firstName, last_name: lastName, email });
      router.back();
    } catch (e) {
      setError(e instanceof ApiClientError ? e.message : 'Impossible d’enregistrer le profil.');
    }
  });

  return (
    <SafeAreaView style={styles.safe} edges={['bottom']}>
      <KeyboardAvoidingView style={styles.flex} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
          <View style={styles.form}>
            <FormInput
              control={control}
              name="firstName"
              rules={nameRules}
              label="Prénom"
              autoComplete="given-name"
              autoCapitalize="words"
              placeholder="Saisissez votre prénom"
            />
            <FormInput
              control={control}
              name="lastName"
              rules={nameRules}
              label="Nom"
              autoComplete="family-name"
              autoCapitalize="words"
              placeholder="Saisissez votre nom"
            />
            <FormInput
              control={control}
              name="email"
              rules={emailRules}
              label="Email"
              keyboardType="email-address"
              autoComplete="email"
              placeholder="Saisissez votre adresse email"
            />
            {error ? <Text style={styles.error}>{error}</Text> : null}
            <Button title="Enregistrer" onPress={onSubmit} loading={formState.isSubmitting} style={styles.cta} />
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
