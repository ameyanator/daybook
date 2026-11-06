import * as Linking from 'expo-linking';
import { StatusBar } from 'expo-status-bar';
import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ActivityIndicator,
  Alert,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  RefreshControl,
  ScrollView,
  SectionList,
  StyleSheet,
  Switch,
  Text,
  TextInput,
  View,
} from 'react-native';
import { SafeAreaProvider, SafeAreaView } from 'react-native-safe-area-context';
import * as api from './src/api';
import type { DaybookEvent, User } from './src/api';
import { AuthProvider, useAuth } from './src/auth';
import { DateTimeField } from './src/DateTimeField';
import { groupByDay, isPast, nextHour, toDateString } from './src/schedule';
import { displayName, ShareScreen } from './src/ShareScreen';
import { SignInScreen } from './src/SignInScreen';
import { colors } from './src/theme';

export default function App() {
  return (
    <SafeAreaProvider>
      <StatusBar style="dark" />
      <AuthProvider>
        <Root />
      </AuthProvider>
    </SafeAreaProvider>
  );
}

/** Pulls the token out of daybook://invite/<token>. */
function inviteToken(url: string | null): string | null {
  if (!url) return null;
  const { hostname, path } = Linking.parse(url);
  const parts = [hostname, ...(path ?? '').split('/')].filter(Boolean);
  return parts[0] === 'invite' && parts[1] ? parts[1] : null;
}

function Root() {
  const { user, ready } = useAuth();
  const url = Linking.useLinkingURL();
  const [pendingInvite, setPendingInvite] = useState<string | null>(null);
  const [openCalendar, setOpenCalendar] = useState<User | null>(null);

  useEffect(() => {
    const token = inviteToken(url);
    if (token) setPendingInvite(token);
  }, [url]);

  // Accept an invite once signed in (immediately, or right after sign-in).
  useEffect(() => {
    if (!user || !pendingInvite) return;
    const token = pendingInvite;
    setPendingInvite(null);
    api.acceptInvite(token).then(
      ({ owner }) => {
        setOpenCalendar(owner);
        Alert.alert('Calendar added', `You can now see ${displayName(owner)}'s calendar.`);
      },
      (e) => Alert.alert("Couldn't open invite", (e as Error).message),
    );
  }, [user, pendingInvite]);

  if (!ready) {
    return (
      <View style={[styles.screen, styles.center]}>
        <ActivityIndicator color={colors.accent} />
      </View>
    );
  }
  if (!user) return <SignInScreen inviteWaiting={!!pendingInvite} />;
  return <Home key={user.id} openCalendar={openCalendar} onOpened={() => setOpenCalendar(null)} />;
}

function Home({ openCalendar, onOpened }: { openCalendar: User | null; onOpened: () => void }) {
  const [sharedWithMe, setSharedWithMe] = useState<User[]>([]);
  // null = my own calendar; otherwise the owner of a calendar shared with me.
  const [viewing, setViewing] = useState<User | null>(null);
  const [shareOpen, setShareOpen] = useState(false);

  const loadShares = useCallback(async () => {
    try {
      const s = await api.getShares();
      setSharedWithMe(s.shared_with_me);
      setViewing((v) => (v && !s.shared_with_me.some((u) => u.id === v.id) ? null : v));
    } catch {}
  }, []);

  useEffect(() => {
    loadShares();
  }, [loadShares]);

  useEffect(() => {
    if (openCalendar) {
      loadShares();
      setViewing(openCalendar);
      onOpened();
    }
  }, [openCalendar, loadShares, onOpened]);

  return (
    <SafeAreaView style={styles.screen} edges={['top', 'left', 'right']}>
      <View style={styles.header}>
        <Text style={styles.heading}>Daybook</Text>
        <Pressable onPress={() => setShareOpen(true)} hitSlop={10} style={styles.shareButton}>
          <Text style={styles.shareButtonText}>Share</Text>
        </Pressable>
      </View>

      {sharedWithMe.length > 0 && (
        <ScrollView horizontal showsHorizontalScrollIndicator={false}
          style={styles.tabsScroll} contentContainerStyle={styles.tabs}>
          <Tab label="Mine" active={!viewing} onPress={() => setViewing(null)} />
          {sharedWithMe.map((u) => (
            <Tab key={u.id} label={displayName(u)} active={viewing?.id === u.id} onPress={() => setViewing(u)} />
          ))}
        </ScrollView>
      )}

      <Calendar key={viewing?.id ?? 'mine'} owner={viewing} />

      <ShareScreen visible={shareOpen} onClose={() => { setShareOpen(false); loadShares(); }}
        onSharesChanged={loadShares} />
    </SafeAreaView>
  );
}

function Tab({ label, active, onPress }: { label: string; active: boolean; onPress: () => void }) {
  return (
    <Pressable onPress={onPress} style={[styles.tab, active && styles.tabActive]}>
      <Text style={[styles.tabText, active && styles.tabTextActive]} numberOfLines={1}>{label}</Text>
    </Pressable>
  );
}

/** A schedule list. `owner` null = mine (editable), otherwise read-only. */
function Calendar({ owner }: { owner: User | null }) {
  const readOnly = !!owner;
  const [events, setEvents] = useState<DaybookEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setEvents(await api.listEvents(owner?.id));
      setError(null);
    } catch (e) {
      setError((e as Error).message);
    }
  }, [owner?.id]);

  useEffect(() => {
    load().finally(() => setLoading(false));
  }, [load]);

  const sections = useMemo(() => groupByDay(events), [events]);

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
            await api.deleteEvent(e.id);
          } catch (err) {
            setEvents(before);
            setError(`Couldn't delete. ${(err as Error).message}`);
          }
        },
      },
    ]);
  }

  const now = new Date();

  return (
    <KeyboardAvoidingView style={styles.flex} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      {readOnly ? (
        <Text style={styles.readOnlyNote}>Viewing {displayName(owner)}'s calendar · read-only</Text>
      ) : (
        <Composer
          onAdd={async (input) => {
            const created = await api.createEvent(input);
            setEvents((prev) => [...prev, created]); // groupByDay keeps it sorted
            setError(null);
          }}
          onError={setError}
        />
      )}

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
            <RefreshControl refreshing={refreshing} onRefresh={async () => {
              setRefreshing(true);
              await load();
              setRefreshing(false);
            }} />
          }
          renderSectionHeader={({ section }) => <Text style={styles.sectionHeader}>{section.title}</Text>}
          renderItem={({ item }) => (
            <Pressable
              onLongPress={readOnly ? undefined : () => confirmDelete(item)}
              style={[styles.item, isPast(item, now) && styles.itemPast]}
            >
              {item.kind === 'allDay' ? (
                <Text style={[styles.itemTime, styles.allDay]}>All day</Text>
              ) : (
                <Text style={styles.itemTime}>
                  {item.startsAt.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}
                </Text>
              )}
              <Text style={styles.itemTitle}>{item.title}</Text>
            </Pressable>
          )}
          ListEmptyComponent={
            <Text style={styles.empty}>
              {readOnly ? 'Nothing on this calendar yet.' : 'Nothing scheduled yet. Add your first thing above.'}
            </Text>
          }
          ListFooterComponent={
            !readOnly && events.length > 0 ? (
              <Text style={styles.hint}>Long-press an item to delete it.</Text>
            ) : undefined
          }
        />
      )}
    </KeyboardAvoidingView>
  );
}

function Composer({ onAdd, onError }: {
  onAdd: (e: api.NewEvent) => Promise<void>;
  onError: (msg: string) => void;
}) {
  const [title, setTitle] = useState('');
  const [when, setWhen] = useState(nextHour);
  const [allDay, setAllDay] = useState(false);
  const [saving, setSaving] = useState(false);
  const canAdd = title.trim().length > 0 && !saving;

  async function add() {
    if (!canAdd) return;
    setSaving(true);
    try {
      await onAdd(
        allDay
          ? { kind: 'allDay', title: title.trim(), date: toDateString(when) }
          : { kind: 'timed', title: title.trim(), startsAt: when },
      );
      setTitle('');
    } catch (e) {
      onError(`Couldn't save. ${(e as Error).message}`);
    } finally {
      setSaving(false);
    }
  }

  return (
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
        <DateTimeField value={when} onChange={setWhen} showTime={!allDay} />
        <View style={styles.allDayToggle}>
          <Text style={styles.allDayLabel}>All day</Text>
          <Switch value={allDay} onValueChange={setAllDay}
            trackColor={{ true: colors.accent, false: colors.chip }} />
        </View>
      </View>
      <Pressable onPress={add} disabled={!canAdd}
        style={[styles.addButton, !canAdd && styles.addButtonDisabled]} accessibilityRole="button">
        <Text style={styles.addButtonText}>{saving ? 'Adding…' : 'Add'}</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1 },
  center: { alignItems: 'center', justifyContent: 'center' },
  screen: { flex: 1, backgroundColor: colors.bg },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 20,
    paddingTop: 12,
    paddingBottom: 12,
  },
  heading: { fontSize: 32, fontWeight: '700', color: colors.text },
  shareButton: { backgroundColor: colors.chip, paddingHorizontal: 14, paddingVertical: 7, borderRadius: 16 },
  shareButtonText: { color: colors.text, fontWeight: '600', fontSize: 15 },
  tabsScroll: { flexGrow: 0, marginBottom: 12 },
  tabs: { paddingHorizontal: 16, gap: 8 },
  tab: {
    paddingHorizontal: 14,
    paddingVertical: 7,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: colors.border,
    maxWidth: 180,
  },
  tabActive: { backgroundColor: colors.text, borderColor: colors.text },
  tabText: { color: colors.text, fontSize: 14, fontWeight: '600' },
  tabTextActive: { color: colors.bg },
  readOnlyNote: { marginHorizontal: 20, color: colors.muted, fontSize: 14 },
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
  allDayToggle: { flexDirection: 'row', alignItems: 'center', gap: 6 },
  allDayLabel: { fontSize: 15, color: colors.muted },
  addButton: { backgroundColor: colors.accent, paddingVertical: 11, borderRadius: 8, alignItems: 'center' },
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
  allDay: { color: colors.muted },
  itemTitle: { flex: 1, fontSize: 16, color: colors.text },
  empty: { textAlign: 'center', color: colors.muted, marginTop: 48, fontSize: 15 },
  hint: { textAlign: 'center', color: colors.muted, marginTop: 16, fontSize: 13 },
});
