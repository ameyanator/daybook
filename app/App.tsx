import { StatusBar } from 'expo-status-bar';
import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ActivityIndicator,
  Alert,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  RefreshControl,
  SectionList,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { SafeAreaProvider, SafeAreaView } from 'react-native-safe-area-context';
import { createEvent, deleteEvent, listEvents, type DaybookEvent } from './src/api';
import { DateTimeField } from './src/DateTimeField';
import { groupByDay, nextHour } from './src/schedule';
import { colors } from './src/theme';

export default function App() {
  return (
    <SafeAreaProvider>
      <StatusBar style="dark" />
      <Daybook />
    </SafeAreaProvider>
  );
}

function Daybook() {
  const [events, setEvents] = useState<DaybookEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [title, setTitle] = useState('');
  const [when, setWhen] = useState(nextHour);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    try {
      setEvents(await listEvents());
      setError(null);
    } catch (e) {
      setError(`Couldn't reach the server. ${(e as Error).message}`);
    }
  }, []);

  useEffect(() => {
    load().finally(() => setLoading(false));
  }, [load]);

  const sections = useMemo(() => groupByDay(events), [events]);
  const canAdd = title.trim().length > 0 && !saving;

  async function add() {
    if (!canAdd) return;
    setSaving(true);
    try {
      const created = await createEvent(title.trim(), when);
      setEvents((prev) => [...prev, created]); // groupByDay keeps it sorted
      setTitle('');
      setError(null);
    } catch (e) {
      setError(`Couldn't save. ${(e as Error).message}`);
    } finally {
      setSaving(false);
    }
  }

  function confirmDelete(e: DaybookEvent) {
    Alert.alert('Delete this?', e.title, [
      { text: 'Cancel', style: 'cancel' },
      {
        text: 'Delete',
        style: 'destructive',
        onPress: async () => {
          const before = events;
          setEvents((prev) => prev.filter((x) => x.id !== e.id));
          try {
            await deleteEvent(e.id);
          } catch (err) {
            setEvents(before);
            setError(`Couldn't delete. ${(err as Error).message}`);
          }
        },
      },
    ]);
  }

  const now = Date.now();

  return (
    <SafeAreaView style={styles.screen} edges={['top', 'left', 'right']}>
      <KeyboardAvoidingView
        style={styles.flex}
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
      >
        <Text style={styles.heading}>Daybook</Text>

        <View style={styles.composer}>
          <TextInput
            style={styles.input}
            placeholder="What's happening?"
            placeholderTextColor={colors.muted}
            value={title}
            onChangeText={setTitle}
            maxLength={200}
            returnKeyType="done"
            onSubmitEditing={add}
          />
          <View style={styles.composerRow}>
            <DateTimeField value={when} onChange={setWhen} />
            <Pressable
              onPress={add}
              disabled={!canAdd}
              style={[styles.addButton, !canAdd && styles.addButtonDisabled]}
              accessibilityRole="button"
            >
              <Text style={styles.addButtonText}>{saving ? '…' : 'Add'}</Text>
            </Pressable>
          </View>
        </View>

        {error && <Text style={styles.error}>{error}</Text>}

        {loading ? (
          <ActivityIndicator style={styles.flex} color={colors.accent} />
        ) : (
          <SectionList
            style={styles.flex}
            contentContainerStyle={styles.listContent}
            sections={sections}
            keyExtractor={(e) => String(e.id)}
            stickySectionHeadersEnabled={false}
            keyboardShouldPersistTaps="handled"
            refreshControl={
              <RefreshControl
                refreshing={refreshing}
                onRefresh={async () => {
                  setRefreshing(true);
                  await load();
                  setRefreshing(false);
                }}
              />
            }
            renderSectionHeader={({ section }) => (
              <Text style={styles.sectionHeader}>{section.title}</Text>
            )}
            renderItem={({ item }) => {
              const past = item.startsAt.getTime() < now;
              return (
                <Pressable
                  onLongPress={() => confirmDelete(item)}
                  style={[styles.item, past && styles.itemPast]}
                >
                  <Text style={styles.itemTime}>
                    {item.startsAt.toLocaleTimeString(undefined, {
                      hour: 'numeric',
                      minute: '2-digit',
                    })}
                  </Text>
                  <Text style={styles.itemTitle}>{item.title}</Text>
                </Pressable>
              );
            }}
            ListEmptyComponent={
              <Text style={styles.empty}>
                Nothing scheduled yet. Add your first thing above.
              </Text>
            }
            ListFooterComponent={
              events.length > 0 ? (
                <Text style={styles.hint}>Long-press an item to delete it.</Text>
              ) : undefined
            }
          />
        )}
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1 },
  screen: { flex: 1, backgroundColor: colors.bg },
  heading: {
    fontSize: 32,
    fontWeight: '700',
    color: colors.text,
    paddingHorizontal: 20,
    paddingTop: 12,
    paddingBottom: 16,
  },
  composer: {
    marginHorizontal: 16,
    padding: 14,
    gap: 12,
    backgroundColor: colors.card,
    borderRadius: 14,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
  },
  input: { fontSize: 17, color: colors.text, paddingVertical: 4 },
  composerRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    flexWrap: 'wrap',
    gap: 8,
  },
  addButton: {
    backgroundColor: colors.accent,
    paddingHorizontal: 20,
    paddingVertical: 9,
    borderRadius: 8,
  },
  addButtonDisabled: { opacity: 0.4 },
  addButtonText: { color: '#fff', fontSize: 16, fontWeight: '600' },
  error: { color: colors.danger, marginHorizontal: 20, marginTop: 12 },
  listContent: { paddingHorizontal: 16, paddingBottom: 40 },
  sectionHeader: {
    fontSize: 14,
    fontWeight: '700',
    color: colors.muted,
    textTransform: 'uppercase',
    letterSpacing: 0.6,
    marginTop: 24,
    marginBottom: 8,
    marginLeft: 4,
  },
  item: {
    flexDirection: 'row',
    alignItems: 'baseline',
    gap: 14,
    paddingVertical: 14,
    paddingHorizontal: 14,
    marginBottom: 8,
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
  },
  itemPast: { opacity: 0.45 },
  itemTime: {
    width: 76,
    fontSize: 15,
    fontWeight: '600',
    color: colors.accent,
    fontVariant: ['tabular-nums'],
  },
  itemTitle: { flex: 1, fontSize: 16, color: colors.text },
  empty: { textAlign: 'center', color: colors.muted, marginTop: 48, fontSize: 15 },
  hint: { textAlign: 'center', color: colors.muted, marginTop: 16, fontSize: 13 },
});
