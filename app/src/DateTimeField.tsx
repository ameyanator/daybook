import DateTimePicker, { DateTimePickerAndroid } from '@react-native-community/datetimepicker';
import { Platform, Pressable, StyleSheet, Text, View } from 'react-native';
import { colors } from './theme';

type Props = { value: Date; onChange: (d: Date) => void };

// iOS shows the native compact pickers inline. Android has no inline picker,
// so we show tappable chips that open the system date/time dialogs.
export function DateTimeField({ value, onChange }: Props) {
  if (Platform.OS === 'ios') {
    return (
      <View style={styles.row}>
        <DateTimePicker mode="date" display="compact" value={value}
          onValueChange={(_, d) => onChange(d)} />
        <DateTimePicker mode="time" display="compact" value={value}
          onValueChange={(_, d) => onChange(d)} />
      </View>
    );
  }

  const open = (mode: 'date' | 'time') =>
    DateTimePickerAndroid.open({ mode, value, onValueChange: (_, d) => onChange(d) });

  return (
    <View style={styles.row}>
      <Chip label={value.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })}
        onPress={() => open('date')} />
      <Chip label={value.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}
        onPress={() => open('time')} />
    </View>
  );
}

function Chip({ label, onPress }: { label: string; onPress: () => void }) {
  return (
    <Pressable onPress={onPress} style={styles.chip} accessibilityRole="button">
      <Text style={styles.chipText}>{label}</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', gap: 8, alignItems: 'center' },
  chip: {
    backgroundColor: colors.chip,
    paddingHorizontal: 14,
    paddingVertical: 8,
    borderRadius: 8,
  },
  chipText: { fontSize: 16, color: colors.text },
});
