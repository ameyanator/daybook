# Daybook

A dead-simple personal schedule. Pick a date (and a time, or mark it "all day"), write one line about what happens, and everything is listed in order, grouped by day. You can share your calendar, read-only, with other people.

```
daybook/
├── app/      Expo (React Native + TypeScript) app for iOS and Android
└── server/   Go HTTP API backed by Postgres
```

## How it works

- **Sign-in:** Google on both platforms, plus Sign in with Apple on iPhone (the App Store requires it when Google sign-in is offered, under guideline 4.8). The app gets a signed ID token from Google or Apple, and the server verifies it and creates its own session. There are no passwords.
- **Data:** everything lives in one Postgres database: Neon's free plan when hosted, and a local Postgres for development. The phone stores only its session token, in the iOS Keychain or Android Keystore.
- **Sharing:** sharing adds a row to the `shares` table that lets another account read your events. Their app fetches your events from the server, so nothing is copied, changes show up for them on refresh, and removing the share cuts off access right away. You can share by email (the address they signed in with) or with a single-use invite link that expires after 7 days.
- **Account deletion:** in the app, under Share → Account → Delete account. This removes the account along with its events, shares and invites. Apple requires in-app deletion (guideline 5.1.1(v)).

## Run it locally

**1. Server** (Go 1.22+, Postgres running locally):

```sh
createdb daybook && createdb daybook_test   # once
cd server
DEV_LOGIN=1 go run .      # :8080; creates/migrates tables on start
go test ./...             # uses the daybook_test database
```

| Env var             | Purpose                                                                 |
|---------------------|-------------------------------------------------------------------------|
| `DATABASE_URL`      | Postgres connection string (default `postgres://localhost:5432/daybook?sslmode=disable`) |
| `ADDR` / `PORT`     | Listen address (default `:8080`). `PORT`, which Render sets, overrides it. |
| `PUBLIC_URL`        | Public base URL, used in invite links (e.g. `https://daybook-xxxx.onrender.com`) |
| `GOOGLE_CLIENT_IDS` | Comma-separated Google OAuth client IDs whose tokens are accepted (the **web** client ID, plus iOS) |
| `APPLE_BUNDLE_IDS`  | Comma-separated iOS bundle IDs for Sign in with Apple (`com.ameyanator.daybook`) |
| `DEV_LOGIN`         | `1` enables a "sign in as any email" endpoint for local testing. **Never enable this in production.** |
| `TEST_DATABASE_URL` | Database used by `go test` (default local `daybook_test`) |

**2. App** (Node 20+):

```sh
cd app
npm install
EXPO_PUBLIC_API_URL=http://$(ipconfig getifaddr en0):8080 npx expo start
```

- **Expo Go** (scan the QR code) works with the dev sign-in only. Google's native SDK isn't included in Expo Go.
- **Real Google/Apple sign-in** needs a development build: `npx expo run:ios` / `npx expo run:android` (requires Xcode / Android Studio), or `npx eas-cli@latest build --profile development`.

## Put it online (free): Render + Neon

1. **Neon** (neon.com): create a project named `daybook` and pick the region nearest your users. Copy the **direct (unpooled)** connection string. If pgx complains about `channel_binding`, remove `&channel_binding=require` from it.
2. **Render** (render.com) → New → Web Service:
   - Connect the GitHub repo `ameyanator/daybook`, with runtime **Docker** (it uses the root `Dockerfile`), the **Free** plan, and health check path `/healthz`.
   - Environment: `DATABASE_URL` = the Neon string, and `PUBLIC_URL` = the service's `https://….onrender.com` address. Add `GOOGLE_CLIENT_IDS` and `APPLE_BUNDLE_IDS` once you have them. Don't set `DEV_LOGIN`.
   - It auto-deploys on every push to `main`. Tables are created on first start.
3. Point the app at it: EAS builds already use the live server (`https://daybook-45ps.onrender.com`, set in `app/eas.json`).

Free-plan caveat: Render puts the server to sleep after 15 minutes idle, and the first request after that takes about a minute while it wakes. Neon also sleeps after 5 minutes but wakes in under a second.

## Shipping app changes

- **JavaScript/UI-only changes** go out over the air, and installed apps pick them up the next time they're opened:
  ```sh
  cd app && npx eas-cli@latest update --channel preview --message "what changed"
  ```
  The bundle reads public config from `app/.env.production`. Keep that file in sync with the `env` blocks in `eas.json`.
- **Native changes** (a new library with native code, app.json plugins or permissions, or an `expo` SDK upgrade) need a new build: `npx eas-cli@latest build -p android --profile preview`. Bump `version` in app.json at the same time, because updates only reach builds with the same `runtimeVersion` (which follows the app version).

## Setting up Google and Apple sign-in

1. **Google Cloud Console → APIs & Services → Credentials.** Create three OAuth client IDs:
   - **Web application.** Its ID goes in `EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID` (app) and `GOOGLE_CLIENT_IDS` (server). The tokens the app sends are issued to this ID.
   - **iOS**, with bundle ID `com.ameyanator.daybook`. Its ID goes in `EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID`. Its reversed form (`com.googleusercontent.apps.…`) goes in `GOOGLE_IOS_URL_SCHEME` at build time.
   - **Android**, with package `com.ameyanator.daybook` and the SHA-1 of your signing key (from `eas credentials`, and later also the Play Console's app-signing key).
   - Also configure the OAuth consent screen.
2. **Apple:** enroll in the Apple Developer Program, then enable the "Sign in with Apple" capability for `com.ameyanator.daybook`. EAS Build does this automatically from `ios.usesAppleSignIn`.
3. Put the app-side values in `app/.env` (git-ignored) or in EAS environment variables.

## API

Every endpoint except `/auth/*`, `/healthz` and `/invite/{token}` needs `Authorization: Bearer <session token>`.

| Method | Path                         | Body / notes                                           |
|--------|------------------------------|--------------------------------------------------------|
| GET    | `/auth/providers`            | which sign-in methods the server has enabled           |
| POST   | `/auth/google`               | `{"id_token"}` → `{"token","user"}`                    |
| POST   | `/auth/apple`                | `{"id_token","name"}` → `{"token","user"}`             |
| POST   | `/auth/logout`               |                                                        |
| GET    | `/me`                        | current user                                           |
| DELETE | `/me`                        | delete account and all its data                        |
| GET    | `/events`                    | my events, in order                                    |
| POST   | `/events`                    | `{"title","starts_at"}` (RFC 3339) **or** `{"title","date"}` (`YYYY-MM-DD`, all day) |
| PUT    | `/events/{id}`               | same body as POST                                      |
| DELETE | `/events/{id}`               |                                                        |
| GET    | `/shares`                    | `{"sharing_with":[…], "shared_with_me":[…]}`           |
| POST   | `/shares`                    | `{"email"}`: share my calendar with that account       |
| DELETE | `/shares/{userId}`           | stop sharing with someone                              |
| DELETE | `/shared-with-me/{userId}`   | stop seeing someone's calendar                         |
| GET    | `/calendars/{userId}/events` | a calendar shared with me (read-only)                  |
| POST   | `/invites`                   | → `{"url","token","expires_at"}`, single use, 7 days   |
| POST   | `/invites/accept`            | `{"token"}`                                            |
| GET    | `/invite/{token}`            | web page with an "Open in Daybook" button              |

## Roadmap to the App Store and Play Store

- [x] Sign in with Google and Apple, with in-app account deletion
- [x] Read-only sharing by email or invite link
- [x] All-day items
- [x] Postgres (Neon) + Docker image for Render
- [ ] **Deploy to Render + Neon** (see "Put it online"). Release builds block plain `http://`.
- [ ] **Universal links / App Links** so that tapping an invite opens the app directly (this needs the server's domain).
- [ ] **Revoke Apple tokens on account deletion.** Apple asks apps to call its REST revoke endpoint, which needs a Sign in with Apple key (`.p8`).
- [ ] Rate limiting on the sign-in and share endpoints
- [ ] Reminders (`expo-notifications`) and editing an event in the UI (the API already supports `PUT`)
- [ ] Offline cache and sync
- [ ] Store assets: icon, splash screen, screenshots, and a privacy policy URL
- [ ] Developer accounts: Apple ($99/year) and Google Play ($25 one-time). New personal Play accounts must run a closed test before production.
- [ ] Build and submit with EAS: `npx eas-cli@latest build --platform all`, then `eas submit`.
