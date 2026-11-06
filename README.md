# Daybook

A dead-simple personal schedule: pick a date and time, write one line about what happens, and everything is listed in time order, grouped by day.

```
daybook/
├── app/      Expo (React Native + TypeScript) app for iOS and Android
└── server/   Go HTTP API backed by SQLite
```

## Run it locally

**1. Start the server** (Go 1.22+):

```sh
cd server
go run .            # listens on :8080, stores data in ./daybook.db
go test ./...
```

Env vars: `ADDR` (default `:8080`), `DB_PATH` (default `daybook.db`).

**2. Start the app** (Node 20+):

```sh
cd app
npm install
EXPO_PUBLIC_API_URL=http://<your-computer-LAN-IP>:8080 npx expo start
```

Scan the QR code with **Expo Go** on your phone (same Wi-Fi as your computer). From a physical phone, `localhost` means the phone itself, so use your computer's LAN IP (`ipconfig getifaddr en0` on macOS). The iOS simulator can use `http://localhost:8080`, and the Android emulator can use `http://10.0.2.2:8080`.

## API

Every request needs an `X-Device-ID` header. The app generates one per install, and it scopes all data to that install.

| Method | Path           | Body                                   | Returns                     |
|--------|----------------|----------------------------------------|-----------------------------|
| GET    | `/events`      | —                                      | events, sorted by time      |
| POST   | `/events`      | `{"title": "...", "starts_at": RFC3339}` | 201 + created event       |
| PUT    | `/events/{id}` | same as POST                           | updated event               |
| DELETE | `/events/{id}` | —                                      | 204                         |
| GET    | `/healthz`     | —                                      | `ok`                        |

## Roadmap to the App Store and Play Store

- [ ] **Real accounts.** Replace the device-ID header with Sign in with Apple and Google. Without it, data is lost when the app is reinstalled. Apple requires Sign in with Apple if any other social login is offered.
- [ ] **Deploy the server over HTTPS.** Fly.io, Railway, or Cloud Run with a persistent volume for SQLite, or move to Postgres. Release builds block plain `http://`.
- [ ] **Reminders.** Local notifications before each event (`expo-notifications`).
- [ ] **Edit an event.** The API already supports `PUT`.
- [ ] **Offline support.** Cache events on the device and sync later.
- [ ] **Store assets.** App icon, splash screen, screenshots, and a privacy policy URL (both stores require one).
- [ ] **Developer accounts.** Apple Developer Program ($99/year) and Google Play Console ($25 one-time). New personal Play accounts must run a closed test with testers before production.
- [ ] **Build and submit with EAS.** `npx eas-cli@latest build --platform all`, then `eas submit`.
