const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function required(label: string) {
  return { required: `${label} requis` as const };
}

export const nameRules = {
  ...required('Ce champ'),
  minLength: { value: 2, message: '2 caractères minimum' },
  maxLength: { value: 80, message: '80 caractères maximum' },
  validate: (value: string) => value.trim().length >= 2 || '2 caractères minimum',
};

export const emailRules = {
  ...required('Email'),
  maxLength: { value: 160, message: 'Email trop long' },
  pattern: { value: EMAIL_RE, message: 'Email invalide' },
};

export const passwordRules = {
  ...required('Mot de passe'),
  minLength: { value: 8, message: '8 caractères minimum' },
  maxLength: { value: 72, message: '72 caractères maximum' },
};

export const loginPasswordRules = {
  ...required('Mot de passe'),
};

export function confirmPasswordRules(password: string) {
  return {
    ...required('Confirmation'),
    validate: (value: string) => value === password || 'Les mots de passe ne correspondent pas',
  };
}
