import * as AppleAuthentication from 'expo-apple-authentication';
import { isRunningInExpoGo } from 'expo';
import { useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Platform,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import * as api from './api';
import { useAuth } from './auth';
import { colors } from './theme';

const GOOGLE_WEB_CLIENT_ID = process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID;
const GOOGLE_IOS_CLIENT_ID = process.env.EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID;

// Google's native SDK isn't in Expo Go — it needs a development build.
const googleUsable = !isRunningInExpoGo() && !!GOOGLE_WEB_CLIENT_ID;

export function SignInScreen({ inviteWaiting }: { inviteWaiting: boolean }) {
  const { completeSignIn } = useAuth();
  const [providers, setProviders] = useState<api.Providers | null>(null);
  const [appleAvailable, setAppleAvailable] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [devEmail, setDevEmail] = useState('');

  useEffect(() => {
    api.getProviders().then(setProviders, (e) => setError(e.message));
    if (Platform.OS === 'ios') AppleAuthentication.isAvailableAsync().then(setAppleAvailable);
  }, []);

  async function run(fn: () => Promise<api.Session | null>) {
    setBusy(true);
    setError(null);
    try {
      const session = await fn();
      if (session) await completeSignIn(session);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const google = () =>
    run(async () => {
      const { GoogleSignin, isErrorWithCode, statusCodes } = await import(
        '@react-native-google-signin/google-signin'
      );
      GoogleSignin.configure({ webClientId: GOOGLE_WEB_CLIENT_ID, iosClientId: GOOGLE_IOS_CLIENT_ID });
      try {
        await GoogleSignin.hasPlayServices();
        const res = await GoogleSignin.signIn();
        if (res.type === 'cancelled') return null;
        if (!res.data.idToken) throw new Error('Google did not return an ID token.');
        return api.signInWithIdToken('google', res.data.idToken);
      } catch (e) {
        if (isErrorWithCode(e) && e.code === statusCodes.SIGN_IN_CANCELLED) return null;
        throw e;
      }
    });

  const apple = () =>
    run(async () => {
      try {
        const cred = await AppleAuthentication.signInAsync({
          requestedScopes: [
            AppleAuthentication.AppleAuthenticationScope.FULL_NAME,
            AppleAuthentication.AppleAuthenticationScope.EMAIL,
          ],
        });
        if (!cred.identityToken) throw new Error('Apple did not return an identity token.');
        // Apple shares the name only on the very first sign-in.
        const name = cred.fullName ? AppleAuthentication.formatFullName(cred.fullName) : undefined;
        return api.signInWithIdToken('apple', cred.identityToken, name);
      } catch (e) {
        if ((e as { code?: string }).code === 'ERR_REQUEST_CANCELED') return null;
        throw e;
      }
    });

  const showGoogle = providers?.google && googleUsable;
  const showApple = providers?.apple && appleAvailable;
  const showDev = __DEV__ && providers?.dev;
  const nothingAvailable = providers && !showGoogle && !showApple && !showDev;

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.hero}>
        <Text style={styles.title}>Daybook</Text>
        <Text style={styles.tagline}>Your day, one line at a time.</Text>
        {inviteWaiting && (
          <Text style={styles.invite}>Sign in to open the calendar you were invited to.</Text>
        )}
      </View>

      <View style={styles.buttons}>
        {!providers && !error && <ActivityIndicator color={colors.accent} />}

        {showApple && (
          <AppleAuthentication.AppleAuthenticationButton
            buttonType={AppleAuthentication.AppleAuthenticationButtonType.CONTINUE}
            buttonStyle={AppleAuthentication.AppleAuthenticationButtonStyle.BLACK}
            cornerRadius={10}
            style={styles.appleButton}
            onPress={apple}
          />
        )}

        {showGoogle && (
          <Pressable style={styles.googleButton} onPress={google} disabled={busy}>
            <Text style={styles.googleG}>G</Text>
            <Text style={styles.googleText}>Continue with Google</Text>
          </Pressable>
        )}

        {showDev && (
          <View style={styles.dev}>
            <Text style={styles.devLabel}>Development sign-in (DEV_LOGIN=1)</Text>
            <TextInput
              style={styles.devInput}
              placeholder="any email"
              placeholderTextColor={colors.muted}
              autoCapitalize="none"
              keyboardType="email-address"
              value={devEmail}
              onChangeText={setDevEmail}
              onSubmitEditing={() => run(() => api.devSignIn(devEmail))}
            />
            <Pressable style={styles.devButton} onPress={() => run(() => api.devSignIn(devEmail))}>
              <Text style={styles.devButtonText}>Sign in</Text>
            </Pressable>
          </View>
        )}

        {nothingAvailable && (
          <Text style={styles.error}>
            {__DEV__ && !providers.dev
              ? 'No sign-in method is available. For local testing, restart the server with:\n\nDEV_LOGIN=1 go run .'
              : 'No sign-in method is available. Google sign-in needs a development build (not Expo Go) and EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID.'}
          </Text>
        )}
        {busy && <ActivityIndicator color={colors.accent} />}
        {error && <Text style={styles.error}>{error}</Text>}
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg, padding: 24, justifyContent: 'space-between' },
  hero: { marginTop: 80, gap: 8 },
  title: { fontSize: 44, fontWeight: '800', color: colors.text },
  tagline: { fontSize: 18, color: colors.muted },
  invite: { fontSize: 15, color: colors.accent, marginTop: 16 },
  buttons: { gap: 12, marginBottom: 24 },
  appleButton: { height: 50, width: '100%' },
  googleButton: {
    height: 50,
    borderRadius: 10,
    backgroundColor: colors.card,
    borderWidth: 1,
    borderColor: colors.border,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 10,
  },
  googleG: { fontSize: 20, fontWeight: '700', color: '#4285F4' },
  googleText: { fontSize: 17, fontWeight: '600', color: colors.text },
  dev: { gap: 8, padding: 12, borderRadius: 10, borderWidth: 1, borderStyle: 'dashed', borderColor: colors.border },
  devLabel: { fontSize: 12, color: colors.muted },
  devInput: { fontSize: 16, color: colors.text, paddingVertical: 6 },
  devButton: { alignSelf: 'flex-start', backgroundColor: colors.chip, paddingHorizontal: 14, paddingVertical: 8, borderRadius: 8 },
  devButtonText: { color: colors.text, fontWeight: '600' },
  error: { color: colors.danger, textAlign: 'center' },
});
