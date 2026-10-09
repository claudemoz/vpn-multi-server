import { Link, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { StyleSheet, Text, View } from 'react-native';

import { AuthScreen } from '@/components/ui/AuthScreen';
import { Button } from '@/components/ui/Button';
import { FormInput } from '@/components/ui/FormInput';
import { Palette, Spacing } from '@/constants/theme';
import { useAuth } from '@/contexts/AuthContext';
import { ApiClientError } from '@/services/api';
import { emailRules, loginPasswordRules } from '@/services/auth-rules';

type LoginValues = {
  email: string;
  password: string;
};

export default function LoginScreen() {
  const { registered } = useLocalSearchParams<{ registered?: string }>();
  const { login } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const { control, handleSubmit, formState, setFocus } = useForm<LoginValues>({
    defaultValues: { email: '', password: '' },
    mode: 'onSubmit',
    reValidateMode: 'onChange',
  });

  const onSubmit = handleSubmit(async ({ email, password }) => {
    setError(null);
    try {
      await login(email, password);
    } catch (e) {
      setError(
        e instanceof ApiClientError
          ? e.message
          : 'Connexion impossible. Vérifiez l’URL de l’API.',
      );
    }
  });

  return (
    <AuthScreen>
      <View style={styles.block}>
        <View style={styles.header}>
          <Text style={styles.title}>Connexion</Text>
          <Text style={styles.subtitle}>Entrez vos identifiants pour continuer.</Text>
        </View>

        {registered === '1' ? (
          <Text style={styles.success}>Compte créé. Vous pouvez vous connecter.</Text>
        ) : null}

        <FormInput
          control={control}
          name="email"
          rules={emailRules}
          label="Email"
          keyboardType="email-address"
          autoComplete="email"
          textContentType="emailAddress"
          placeholder="Saisissez votre adresse email"
          returnKeyType="next"
          onSubmitEditing={() => setFocus('password')}
          submitBehavior="submit"
        />
        <FormInput
          control={control}
          name="password"
          rules={loginPasswordRules}
          label="Mot de passe"
          secureTextEntry
          autoComplete="password"
          textContentType="password"
          placeholder="Saisissez votre mot de passe"
          returnKeyType="go"
          onSubmitEditing={onSubmit}
        />

        {error ? <Text style={styles.error}>{error}</Text> : null}

        <Button
          title="Se connecter"
          onPress={onSubmit}
          loading={formState.isSubmitting}
          style={styles.cta}
        />

        <View style={styles.footer}>
          <Text style={styles.footerText}>Pas de compte ?</Text>
          <Link href="/(auth)/register" asChild>
            <Text style={styles.link}>Créer un compte</Text>
          </Link>
        </View>
      </View>
    </AuthScreen>
  );
}

const styles = StyleSheet.create({
  block: {
    gap: Spacing.three,
  },
  header: {
    marginBottom: Spacing.two,
    gap: 6,
  },
  title: {
    fontSize: 26,
    fontWeight: '600',
    color: '#fff',
  },
  subtitle: {
    fontSize: 15,
    color: Palette.muted,
  },
  cta: {
    marginTop: Spacing.one,
    borderRadius: 12,
    shadowOpacity: 0,
    elevation: 0,
  },
  success: {
    color: Palette.green,
    fontSize: 14,
  },
  error: {
    color: '#FCA5A5',
    fontSize: 14,
  },
  footer: {
    marginTop: Spacing.two,
    flexDirection: 'row',
    justifyContent: 'center',
    gap: Spacing.one,
  },
  footerText: {
    color: Palette.muted,
    fontSize: 15,
  },
  link: {
    color: Palette.blue,
    fontSize: 15,
    fontWeight: '600',
  },
});
