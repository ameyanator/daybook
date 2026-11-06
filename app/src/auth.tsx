import * as SecureStore from 'expo-secure-store';
import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react';
import * as api from './api';
import type { User } from './api';

const TOKEN_KEY = 'daybook.session';

type AuthState = {
  user: User | null;
  ready: boolean; // false while restoring a saved session at launch
  completeSignIn: (session: api.Session) => Promise<void>;
  signOut: () => Promise<void>;
  deleteAccount: () => Promise<void>;
};

const AuthContext = createContext<AuthState | null>(null);

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider');
  return ctx;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [ready, setReady] = useState(false);

  const clearLocal = useCallback(async () => {
    api.setSession(null);
    setUser(null);
    await SecureStore.deleteItemAsync(TOKEN_KEY);
    // Also forget the Google account choice so the picker shows next time.
    await signOutOfProviders();
  }, []);

  // Restore the saved session on launch.
  useEffect(() => {
    (async () => {
      try {
        const token = await SecureStore.getItemAsync(TOKEN_KEY);
        if (token) {
          api.setSession(token, clearLocal);
          setUser(await api.getMe());
        }
      } catch (e) {
        // A 401 already cleared the session. Offline: stay signed out for
        // now; the token is kept and retried next launch.
        if (!(e instanceof api.ApiError && e.status === 0)) await clearLocal();
        else api.setSession(null);
      } finally {
        setReady(true);
      }
    })();
  }, [clearLocal]);

  const completeSignIn = useCallback(
    async (session: api.Session) => {
      await SecureStore.setItemAsync(TOKEN_KEY, session.token);
      api.setSession(session.token, clearLocal);
      setUser(session.user);
    },
    [clearLocal],
  );

  const signOut = useCallback(async () => {
    try {
      await api.logout();
    } catch {}
    await clearLocal();
  }, [clearLocal]);

  const deleteAccount = useCallback(async () => {
    await api.deleteAccount();
    await clearLocal();
  }, [clearLocal]);

  return (
    <AuthContext.Provider value={{ user, ready, completeSignIn, signOut, deleteAccount }}>
      {children}
    </AuthContext.Provider>
  );
}

async function signOutOfProviders() {
  try {
    const { GoogleSignin } = await import('@react-native-google-signin/google-signin');
    await GoogleSignin.signOut();
  } catch {
    // Not available (e.g. Expo Go) or never signed in.
  }
}
