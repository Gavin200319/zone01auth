# Z01auth backend

Go (Fiber) + Firebase Realtime Database REST API for the Z01auth prototype: Google sign-in with device binding, proximity-gated biometrics (campus Wi-Fi BSSID + GPS geofence), first-seen / last-seen attendance, history and settings.

## Run

1. Firebase console > Build > Realtime Database > create the database, copy its URL into `DATABASE_URL`.
2. Project settings > Service accounts > Generate new private key. Save it as `service-account.json` in this folder (it is git-ignored) and set `FIREBASE_CREDENTIALS_FILE=./service-account.json`.
3. Realtime Database > Rules: paste `database.rules.json` and publish. The backend uses the Admin SDK, which bypasses rules, so locking everything down means phones can never read or write the database directly.
4. Then:

```bash
cp .env.example .env        # or add the two Firebase lines to your existing .env
go mod tidy
make fmt
make run                    # or: docker compose up --build
go test ./...
```

For local testing without Google, set `DEV_AUTH=true` and send `id_token: "dev:alex.b@zone01.kisumu.ke"`. The server refuses to start with `DEV_AUTH=true` and `APP_ENV=production`.

## Data layout (Realtime Database)

Everything is addressed by key, so no `.indexOn` rules are needed.

| Path | Holds |
|---|---|
| `users/{uid}` | profile and `settings` |
| `google_subs/{sha256(sub)}` | uid lookup |
| `devices/{rowId}`, `device_index/{deviceId}`, `user_devices/{uid}/{rowId}` | device binding (one device, one account) |
| `sessions/{sid}`, `refresh_index/{hash}`, `device_sessions/{rowId}/{sid}` | refresh sessions |
| `challenges/{id}` | single-use biometric challenges |
| `premises/{id}` | campus, radii, BSSIDs (seeded from env at startup) |
| `attendance/{uid}/{YYYY-MM-DD}` | first seen, last seen, verified |
| `attendance_months/{uid}/{YYYY-MM}` | months with records |
| `events/{uid}/{ms}-{rand}` | recent detections, time-ordered keys |

First-seen, last-seen, refresh rotation, challenge use and settings use database transactions, so concurrent heartbeats cannot corrupt a day. Expired challenges and old events are not purged automatically.

## How the prototype maps to the API

| Prototype | Backend |
|---|---|
| Frame 1 proximity banner (NEAR / OFF PREMISES, distance, Wi-Fi detected) | `POST /proximity/check` |
| Biometric sign-in (on premises only) | `POST /auth/biometric/challenge` then `POST /auth/biometric/login` |
| Continue with Google (device binding) | `POST /auth/google` |
| Frame 2 permissions | client only; `GET /config` gives geofence and polling interval |
| Frame 3 radar (checked_in / near_premises / off_premises), pills, first/last seen, recent detections | `POST /attendance/heartbeat`, `GET /attendance/today` |
| Biometric Verify & Sync button | `POST /attendance/biometric/challenge` then `POST /attendance/heartbeat` with `biometric` |
| Settings: theme, background polling, proximity lock, email, initials | `GET /me`, `PATCH /me/settings` |
| Sign out | `POST /auth/logout` |
| Frame 4 history (month picker, present, avg hours, verified %, daily cards) | `GET /attendance/months`, `GET /attendance/history?month=YYYY-MM` |

## Rules implemented

- **On premises** = connected to a registered BSSID, or GPS inside the geofence (default 50 m, accuracy must be <= 100 m). Between 50 and 150 m the zone is `approaching` and still locked; beyond that `off_premises`. Android placeholder BSSID `02:00:00:00:00:00` is ignored; `is_mock: true` is rejected.
- **Sign-in is gated**: Google sign-in, biometric login and their challenges return `403 PROXIMITY_REQUIRED` off premises.
- **Time In is locked for the day**: the first on-premises scan creates the day record and `first_seen_at` never changes. With `REQUIRE_BIOMETRIC_TIME_IN=true` (default) the first scan must carry a verified biometric; until then heartbeats return `state: near_premises`, `check_in_required: true`.
- **Time Out = last seen**: every on-premises heartbeat after check-in moves `last_seen_at` forward (never backward). Off-premises heartbeats change nothing.
- **Day boundary** uses `TIMEZONE` (default `Africa/Nairobi`); the server clock is authoritative.
- **Device binding**: a `device_id` belongs to one account. Another account signing in on it gets `409 DEVICE_BOUND`. Users free a device with `DELETE /me/devices/:id`.

## Biometrics

A server cannot read fingerprints, so the phone proves it. The app creates an EC P-256 key in the Android Keystore with `setUserAuthenticationRequired(true)`, sends the public key (base64 X.509) at Google sign-in (`device.biometric_public_key`) or via `PUT /me/device/biometric-key`, and signs challenges after a successful `BiometricPrompt`:

1. Request a challenge (`nonce`, valid 2 minutes, single use).
2. Sign the UTF-8 bytes of `nonce` with `SHA256withECDSA` (DER signature), base64-encode it.
3. Send `challenge_id` + `signature`.

## Endpoints (`/api/v1`)

Errors are `{"error":{"code","message"}}`. Authenticated routes need `Authorization: Bearer <access_token>`.

`location` object used below: `{"lat":-0.1,"lng":34.75,"accuracy_m":8,"bssid":"00:1a:2b:3c:4d:5e","ssid":"Z01_Campus_5G","is_mock":false}` (all fields optional, lat/lng together).

| Method | Path | Body / notes |
|---|---|---|
| GET | `/config` | premises, geofence radii, timezone, `heartbeat_interval_seconds` |
| POST | `/proximity/check` | `{location}` -> `zone, distance_m, wifi_status, gps_status, biometrics_unlocked` |
| POST | `/auth/google` | `{id_token, device:{device_id,platform,name,biometric_public_key?}, location}` |
| POST | `/auth/biometric/challenge` | `{device_id, location}` |
| POST | `/auth/biometric/login` | `{device_id, challenge_id, signature, location}` |
| POST | `/auth/refresh` | `{refresh_token}` (rotating) |
| POST | `/auth/logout` | revokes the session, 204 |
| GET | `/me` | user (with `initials`), settings, device |
| PATCH | `/me/settings` | `{theme?: "dark"\|"light", background_polling?: bool}` |
| PUT | `/me/device/biometric-key` | `{public_key}` |
| GET / DELETE | `/me/devices`, `/me/devices/:id` | list / unbind |
| GET | `/attendance/today` | check-in state, `first_seen_at`, `last_seen_at`, `active_seconds`, `recent_events` |
| POST | `/attendance/biometric/challenge` | challenge for check-in |
| POST | `/attendance/heartbeat` | `{location, source:"auto"\|"manual", biometric?:{challenge_id,signature}}` -> `radar` + `today` |
| GET | `/attendance/months` | months with records |
| GET | `/attendance/history?month=2026-10` | `summary{present_days,avg_hours,verified_percent}` + `days[]` |

`/health` is outside `/api/v1`.

## Client flow

1. Launch: `GET /config`, then `POST /proximity/check` for the Frame 1 banner.
2. Sign in (Google or biometric), store both tokens, `POST /auth/refresh` on 401.
3. Every `heartbeat_interval_seconds` while `background_polling` is on: `POST /attendance/heartbeat` with `source:"auto"`. Render `radar.state`, `wifi_status`, `gps_status`, `biometric_verified`.
4. Biometric Verify button: challenge, `BiometricPrompt`, sign, heartbeat with `source:"manual"` and `biometric`.

## Not included

- No admin or reporting endpoints (premises and BSSIDs are seeded from env on startup).
- The original blueprint PDF contract was not available, so this API follows the prototype; paths and field names are easy to adjust.
