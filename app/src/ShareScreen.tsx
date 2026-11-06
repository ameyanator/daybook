import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Alert,
  Modal,
  Pressable,
  ScrollView,
  Share,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import * as api from './api';
import type { User } from './api';
import { useAuth } from './auth';
import { colors } from './theme';

type Props = { visible: boolean; onClose: () => void; onSharesChanged: () => void };

export const displayName = (u: User) => u.name || u.email || 'Someone';

export function ShareScreen({ visible, onClose, onSharesChanged }: Props) {
  const { user, signOut, deleteAccount } = useAuth();
  const [shares, setShares] = useState<api.Shares | null>(null);
  const [email, setEmail] = useState('');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<{ text: string; error?: boolean } | null>(null);

  const load = useCallback(async () => {
    try {
      setShares(await api.getShares());
    } catch (e) {
      setMessage({ text: (e as Error).message, error: true });
    }
  }, []);

  useEffect(() => {
    if (visible) {
      setMessage(null);
      load();
    }
  }, [visible, load]);

  async function shareByEmail() {
    if (!email.trim()) return;
    setBusy(true);
    setMessage(null);
    try {
      const users = await api.shareWithEmail(email);
      setMessage({ text: `Shared with ${users.map(displayName).join(', ')}.` });
      setEmail('');
      await load();
    } catch (e) {
      setMessage({ text: (e as Error).message, error: true });
    } finally {
      setBusy(false);
    }
  }

  async function sendInvite() {
    setBusy(true);
    setMessage(null);
    try {
      const { url } = await api.createInvite();
      await Share.share({
        message: `${displayName(user!)} shared their Daybook calendar with you: ${url}`,
      });
    } catch (e) {
      setMessage({ text: (e as Error).message, error: true });
    } finally {
      setBusy(false);
    }
  }

  function confirm(title: string, body: string, action: string, fn: () => Promise<unknown>) {
    Alert.alert(title, body, [
      { text: 'Cancel', style: 'cancel' },
      {
        text: action,
        style: 'destructive',
        onPress: async () => {
          try {
            await fn();
            await load();
            onSharesChanged();
          } catch (e) {
            setMessage({ text: (e as Error).message, error: true });
          }
        },
      },
    ]);
  }

  return (
    <Modal visible={visible} animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <SafeAreaView style={styles.screen} edges={['top', 'bottom', 'left', 'right']}>
        <View style={styles.header}>
          <Text style={styles.heading}>Sharing</Text>
          <Pressable onPress={onClose} hitSlop={12}>
            <Text style={styles.done}>Done</Text>
          </Pressable>
        </View>

        <ScrollView contentContainerStyle={styles.body} keyboardShouldPersistTaps="handled">
          <Text style={styles.explain}>
            People you share with can see your schedule. They can't change it.
          </Text>

          <View style={styles.card}>
            <Text style={styles.label}>Share with an email</Text>
            <TextInput
              style={styles.input}
              placeholder="the email they signed in with"
              placeholderTextColor={colors.muted}
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="email-address"
              value={email}
              onChangeText={setEmail}
              onSubmitEditing={shareByEmail}
            />
            <Pressable style={[styles.primary, (!email.trim() || busy) && styles.disabled]}
              onPress={shareByEmail} disabled={!email.trim() || busy}>
              <Text style={styles.primaryText}>Share</Text>
            </Pressable>
            <View style={styles.divider} />
            <Text style={styles.label}>Or send an invite link</Text>
            <Text style={styles.small}>
              Works even if they haven't joined yet. The link can be used once and expires in 7 days.
            </Text>
            <Pressable style={[styles.secondary, busy && styles.disabled]} onPress={sendInvite} disabled={busy}>
              <Text style={styles.secondaryText}>Send invite link…</Text>
            </Pressable>
          </View>

          {busy && <ActivityIndicator color={colors.accent} />}
          {message && (
            <Text style={[styles.message, message.error && styles.messageError]}>{message.text}</Text>
          )}

          <Section title="People who can see your calendar">
            {shares?.sharing_with.length === 0 && <Text style={styles.small}>No one yet.</Text>}
            {shares?.sharing_with.map((u) => (
              <PersonRow key={u.id} user={u} action="Remove"
                onAction={() => confirm('Stop sharing?', `${displayName(u)} won't see your calendar anymore.`,
                  'Stop sharing', () => api.stopSharing(u.id))} />
            ))}
          </Section>

          <Section title="Calendars shared with you">
            {shares?.shared_with_me.length === 0 && <Text style={styles.small}>None yet.</Text>}
            {shares?.shared_with_me.map((u) => (
              <PersonRow key={u.id} user={u} action="Leave"
                onAction={() => confirm('Remove this calendar?', `You'll stop seeing ${displayName(u)}'s calendar.`,
                  'Remove', () => api.leaveCalendar(u.id))} />
            ))}
          </Section>

          <Section title="Account">
            <Text style={styles.small}>Signed in as {user?.email || displayName(user!)}</Text>
            <Pressable style={styles.secondary} onPress={() => { onClose(); signOut(); }}>
              <Text style={styles.secondaryText}>Sign out</Text>
            </Pressable>
            <Pressable
              onPress={() =>
                Alert.alert(
                  'Delete your account?',
                  'This permanently deletes your account, all your entries, and everything you have shared. This cannot be undone.',
                  [
                    { text: 'Cancel', style: 'cancel' },
                    {
                      text: 'Delete account',
                      style: 'destructive',
                      onPress: async () => {
                        try {
                          await deleteAccount();
                          onClose();
                        } catch (e) {
                          setMessage({ text: (e as Error).message, error: true });
                        }
                      },
                    },
                  ],
                )
              }
            >
              <Text style={styles.danger}>Delete account</Text>
            </Pressable>
          </Section>
        </ScrollView>
      </SafeAreaView>
    </Modal>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <View style={styles.section}>
      <Text style={styles.sectionTitle}>{title}</Text>
      {children}
    </View>
  );
}

function PersonRow({ user, action, onAction }: { user: User; action: string; onAction: () => void }) {
  return (
    <View style={styles.person}>
      <View style={styles.flex}>
        <Text style={styles.personName}>{displayName(user)}</Text>
        {!!user.name && !!user.email && <Text style={styles.small}>{user.email}</Text>}
      </View>
      <Pressable onPress={onAction} hitSlop={8}>
        <Text style={styles.danger}>{action}</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1 },
  screen: { flex: 1, backgroundColor: colors.bg },
  header: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    paddingHorizontal: 20,
    paddingVertical: 12,
  },
  heading: { fontSize: 28, fontWeight: '700', color: colors.text },
  done: { fontSize: 17, fontWeight: '600', color: colors.accent },
  body: { padding: 16, gap: 16, paddingBottom: 48 },
  explain: { fontSize: 15, color: colors.muted },
  card: {
    padding: 14,
    gap: 10,
    backgroundColor: colors.card,
    borderRadius: 14,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
  },
  label: { fontSize: 15, fontWeight: '600', color: colors.text },
  input: {
    fontSize: 16,
    color: colors.text,
    paddingVertical: 8,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
  },
  primary: { alignSelf: 'flex-start', backgroundColor: colors.accent, paddingHorizontal: 18, paddingVertical: 9, borderRadius: 8 },
  primaryText: { color: '#fff', fontWeight: '600', fontSize: 16 },
  secondary: { alignSelf: 'flex-start', backgroundColor: colors.chip, paddingHorizontal: 16, paddingVertical: 9, borderRadius: 8 },
  secondaryText: { color: colors.text, fontWeight: '600', fontSize: 16 },
  disabled: { opacity: 0.4 },
  divider: { height: StyleSheet.hairlineWidth, backgroundColor: colors.border, marginVertical: 4 },
  small: { fontSize: 13, color: colors.muted },
  message: { color: colors.text, textAlign: 'center' },
  messageError: { color: colors.danger },
  section: { gap: 10, marginTop: 8 },
  sectionTitle: { fontSize: 13, fontWeight: '700', color: colors.muted, textTransform: 'uppercase', letterSpacing: 0.6 },
  person: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    padding: 12,
    backgroundColor: colors.card,
    borderRadius: 10,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
  },
  personName: { fontSize: 16, color: colors.text },
  danger: { color: colors.danger, fontWeight: '600', fontSize: 15 },
});
