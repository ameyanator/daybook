import AsyncStorage from '@react-native-async-storage/async-storage';
import * as Crypto from 'expo-crypto';

// Point this at your Go server. On a physical phone, "localhost" is the phone
// itself — use your computer's LAN IP instead, e.g. http://192.168.1.20:8080.
const API_URL = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080';

export type DaybookEvent = {
  id: number;
  title: string;
  startsAt: Date;
};

type WireEvent = { id: number; title: string; starts_at: string };

const fromWire = (e: WireEvent): DaybookEvent => ({
  id: e.id,
  title: e.title,
  startsAt: new Date(e.starts_at),
});

const DEVICE_KEY = 'daybook.deviceId';
let deviceIdPromise: Promise<string> | null = null;

// Each install gets a random ID that scopes its events on the server.
// Placeholder until real sign-in exists.
function getDeviceId(): Promise<string> {
  deviceIdPromise ??= (async () => {
    const existing = await AsyncStorage.getItem(DEVICE_KEY);
    if (existing) return existing;
    const id = Crypto.randomUUID();
    await AsyncStorage.setItem(DEVICE_KEY, id);
    return id;
  })();
  return deviceIdPromise;
}

async function request(path: string, init: RequestInit = {}): Promise<Response> {
  const res = await fetch(API_URL + path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      'X-Device-ID': await getDeviceId(),
      ...init.headers,
    },
  });
  if (!res.ok) {
    let message = `Request failed (${res.status})`;
    try {
      message = (await res.json()).error ?? message;
    } catch {}
    throw new Error(message);
  }
  return res;
}

export async function listEvents(): Promise<DaybookEvent[]> {
  const res = await request('/events');
  return ((await res.json()) as WireEvent[]).map(fromWire);
}

export async function createEvent(title: string, startsAt: Date): Promise<DaybookEvent> {
  const res = await request('/events', {
    method: 'POST',
    body: JSON.stringify({ title, starts_at: startsAt.toISOString() }),
  });
  return fromWire(await res.json());
}

export async function deleteEvent(id: number): Promise<void> {
  await request(`/events/${id}`, { method: 'DELETE' });
}
