// Point this at your Go server. On a physical phone, "localhost" is the phone
// itself — use your computer's LAN IP instead, e.g. http://192.168.1.20:8080.
export const API_URL = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080';

export type User = { id: number; name: string; email: string };

/** A timed item has `startsAt`; an all-day item has `date` ("YYYY-MM-DD"). */
export type DaybookEvent =
  | { id: number; title: string; kind: 'timed'; startsAt: Date }
  | { id: number; title: string; kind: 'allDay'; date: string };

export type NewEvent = { title: string } & (
  | { kind: 'timed'; startsAt: Date }
  | { kind: 'allDay'; date: string }
);

type WireEvent = { id: number; title: string; starts_at?: string; date?: string };

const fromWire = (e: WireEvent): DaybookEvent =>
  e.date
    ? { id: e.id, title: e.title, kind: 'allDay', date: e.date }
    : { id: e.id, title: e.title, kind: 'timed', startsAt: new Date(e.starts_at!) };

export class ApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message);
  }
}

let sessionToken: string | null = null;
let onUnauthorized: () => void = () => {};

export function setSession(token: string | null, unauthorized?: () => void) {
  sessionToken = token;
  if (unauthorized) onUnauthorized = unauthorized;
}

async function request(path: string, init: RequestInit = {}): Promise<Response> {
  let res: Response;
  try {
    res = await fetch(API_URL + path, {
      ...init,
      headers: {
        'Content-Type': 'application/json',
        ...(sessionToken && { Authorization: `Bearer ${sessionToken}` }),
        ...init.headers,
      },
    });
  } catch {
    throw new ApiError("Can't reach the server. Check your connection.", 0);
  }
  if (!res.ok) {
    let message = `Request failed (${res.status})`;
    try {
      message = (await res.json()).error ?? message;
    } catch {}
    if (res.status === 401 && sessionToken) onUnauthorized();
    throw new ApiError(message, res.status);
  }
  return res;
}

const json = <T>(res: Response) => res.json() as Promise<T>;

// ---- auth ----

export type Providers = { google: boolean; apple: boolean; dev: boolean };
export type Session = { token: string; user: User };

export const getProviders = () => request('/auth/providers').then(json<Providers>);

export const signInWithIdToken = (provider: 'google' | 'apple', idToken: string, name?: string) =>
  request(`/auth/${provider}`, {
    method: 'POST',
    body: JSON.stringify({ id_token: idToken, name }),
  }).then(json<Session>);

export const devSignIn = (email: string) =>
  request('/auth/dev', { method: 'POST', body: JSON.stringify({ email }) }).then(json<Session>);

export const getMe = () => request('/me').then(json<User>);
export const logout = () => request('/auth/logout', { method: 'POST' });
export const deleteAccount = () => request('/me', { method: 'DELETE' });

// ---- events ----

export async function listEvents(calendarOwnerId?: number): Promise<DaybookEvent[]> {
  const path = calendarOwnerId ? `/calendars/${calendarOwnerId}/events` : '/events';
  return (await request(path).then(json<WireEvent[]>)).map(fromWire);
}

export async function createEvent(e: NewEvent): Promise<DaybookEvent> {
  const body =
    e.kind === 'timed'
      ? { title: e.title, starts_at: e.startsAt.toISOString() }
      : { title: e.title, date: e.date };
  return fromWire(
    await request('/events', { method: 'POST', body: JSON.stringify(body) }).then(json<WireEvent>),
  );
}

export const deleteEvent = (id: number) => request(`/events/${id}`, { method: 'DELETE' });

// ---- sharing ----

export type Shares = { sharing_with: User[]; shared_with_me: User[] };

export const getShares = () => request('/shares').then(json<Shares>);

export const shareWithEmail = (email: string) =>
  request('/shares', { method: 'POST', body: JSON.stringify({ email }) }).then(json<User[]>);

export const stopSharing = (viewerId: number) =>
  request(`/shares/${viewerId}`, { method: 'DELETE' });

export const leaveCalendar = (ownerId: number) =>
  request(`/shared-with-me/${ownerId}`, { method: 'DELETE' });

export const createInvite = () =>
  request('/invites', { method: 'POST' }).then(json<{ url: string; expires_at: string }>);

export const acceptInvite = (token: string) =>
  request('/invites/accept', { method: 'POST', body: JSON.stringify({ token }) }).then(
    json<{ owner: User }>,
  );
