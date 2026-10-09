import { Link, router } from 'expo-router';
import { useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { Keyboard, StyleSheet, Text, View, type ScrollView as ScrollViewType } from 'react-native';

import { AuthScreen } from '@/components/ui/AuthScreen';
import { Button } from '@/components/ui/Button';
import { FormInput } from '@/components/ui/FormInput';
import { Palette, Spacing } from '@/constants/theme';
import { useAuth } from '@/contexts/AuthContext';
import { ApiClientError } from '@/services/api';
import { confirmPasswordRules, emailRules, nameRules, passwordRules } from '@/services/auth-rules';

type RegisterValues = {
  firstName: string;
  lastName: string;
  email: string;
  password: string;
  confirmPassword: string;
};

export default function RegisterScreen() {
  const scrollRef = useRef<ScrollViewType>(null);
  const { register } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const { control, handleSubmit, formState, watch, trigger } = useForm<RegisterValues>({
    defaultValues: {
      firstName: '',
      lastName: '',
      email: '',
      password: '',
      confirmPassword: '',
    },
    mode: 'onSubmit',
    reValidateMode: 'onChange',
  });
  const password = watch('password');

  const onSubmit = handleSubmit(async ({ firstName, lastName, email, password }) => {
    Keyboard.dismiss();
    setError(null);
    try {
      await register({
        first_name: firstName,
        last_name: lastName,
        email,
        password,
      });
      router.replace({ pathname: '/(auth)/login', params: { registered: '1' } });
    } catch (e) {
      setError(
        e instanceof ApiClientError
          ? e.message
          : e instanceof Error
            ? e.message
            : 'Inscription impossible. Vérifiez l’URL de l’API.',
      );
      scrollRef.current?.scrollToEnd({ animated: true });
    }
  });

  return (
    <AuthScreen scrollRef={scrollRef}>
      <View style={styles.block}>
        <View style={styles.header}>
          <Text style={styles.title}>Créer un compte</Text>
          <Text style={styles.subtitle}>Renseignez vos informations pour créer un compte.</Text>
        </View>

        <View style={styles.row}>
          <View style={styles.half}>
            <FormInput
              control={control}
              name="firstName"
              rules={nameRules}
              label="Prénom"
              autoComplete="given-name"
              autoCapitalize="words"
              placeholder="Saisissez votre prénom"
            />
          </View>
          <View style={styles.half}>
            <FormInput
              control={control}
              name="lastName"
              rules={nameRules}
              label="Nom"
              autoComplete="family-name"
              autoCapitalize="words"
              placeholder="Saisissez votre nom"
            />
          </View>
        </View>

        <FormInput
          control={control}
          name="email"
          rules={emailRules}
          label="Email"
          keyboardType="email-address"
          autoComplete="email"
          placeholder="Saisissez votre adresse email"
        />
        <FormInput
          control={control}
          name="password"
          rules={passwordRules}
          label="Mot de passe"
          secureTextEntry
          autoComplete="new-password"
          placeholder="Saisissez un mot de passe (8 caractères min.)"
          onChangeText={() => {
            if (formState.dirtyFields.confirmPassword) {
              trigger('confirmPassword');
            }
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

        <Button
          title="S'inscrire"
          onPress={onSubmit}
          loading={formState.isSubmitting}
          style={styles.cta}
        />

        <View style={styles.footer}>
          <Text style={styles.footerText}>Déjà un compte ?</Text>
          <Link href="/(auth)/login" asChild>
            <Text style={styles.link}>Se connecter</Text>
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
  row: {
    flexDirection: 'row',
    gap: Spacing.two,
  },
  half: {
    flex: 1,
  },
  cta: {
    marginTop: Spacing.one,
    borderRadius: 12,
    shadowOpacity: 0,
    elevation: 0,
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
