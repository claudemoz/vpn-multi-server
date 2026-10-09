import {
  Controller,
  type Control,
  type FieldPath,
  type FieldValues,
  type RegisterOptions,
} from 'react-hook-form';
import type { TextInputProps } from 'react-native';

import { Input } from '@/components/ui/Input';

type Props<T extends FieldValues> = Omit<TextInputProps, 'value'> & {
  control: Control<T>;
  name: FieldPath<T>;
  label: string;
  rules?: RegisterOptions<T, FieldPath<T>>;
};

export function FormInput<T extends FieldValues>({
  control,
  name,
  label,
  rules,
  onChangeText,
  ...rest
}: Props<T>) {
  return (
    <Controller
      control={control}
      name={name}
      rules={rules}
      render={({ field: { onChange, onBlur, value, ref }, fieldState: { error } }) => (
        <Input
          ref={ref}
          label={label}
          value={value}
          onBlur={onBlur}
          error={error?.message}
          onChangeText={(text) => {
            onChange(text);
            onChangeText?.(text);
          }}
          {...rest}
        />
      )}
    />
  );
}
