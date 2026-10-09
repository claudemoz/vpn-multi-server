import Ionicons from '@expo/vector-icons/Ionicons';

type Props = {
  name: 'home' | 'account';
  color: string;
  focused?: boolean;
};

const ICONS = {
  home: { on: 'home', off: 'home-outline' },
  account: { on: 'person-circle', off: 'person-circle-outline' },
} as const;

export function TabIcon({ name, color, focused }: Props) {
  const icon = ICONS[name];
  return <Ionicons name={focused ? icon.on : icon.off} size={26} color={color} />;
}
