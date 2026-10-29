#!/usr/bin/env bash
cd "$(dirname "$0")"
BASE=${BASE:-http://localhost:8080}
API=$BASE/api/v1
envv() { grep -E "^$1=" .env | head -1 | cut -d= -f2- | tr -d '\r"'; }

LAT=$(envv PREMISES_LAT); LNG=$(envv PREMISES_LNG)
SSID=$(envv PREMISES_SSID); BSSID=$(envv PREMISES_BSSIDS | cut -d, -f1)
DOMAIN=$(envv ALLOWED_EMAIL_DOMAINS | cut -d, -f1); DOMAIN=${DOMAIN:-zone01.kisumu.ke}
ON="{\"lat\":$LAT,\"lng\":$LNG,\"accuracy_m\":8,\"bssid\":\"$BSSID\",\"ssid\":\"$SSID\"}"
OFF='{"lat":10.0,"lng":10.0,"accuracy_m":8}'
MOCK="{\"lat\":$LAT,\"lng\":$LNG,\"accuracy_m\":8,\"is_mock\":true}"
RUN=$(date +%s)
EMAIL="smoke$RUN@$DOMAIN"; DEV="smoke-device-$RUN"

openssl ecparam -name prime256v1 -genkey -noout -out /tmp/smoke.key
PUB=$(openssl ec -in /tmp/smoke.key -pubout -outform DER 2>/dev/null | base64 | tr -d '\n')
sign() { printf %s "$1" | openssl dgst -sha256 -sign /tmp/smoke.key | base64 | tr -d '\n'; }
j() { jq -r "$1" /tmp/smoke.out; }

req() { # name want method path token body
  local args=(-s -o /tmp/smoke.out -w '%{http_code}' -X "$3" "$API$4")
  [ -n "${5:-}" ] && args+=(-H "Authorization: Bearer $5")
  [ -n "${6:-}" ] && args+=(-H 'Content-Type: application/json' -d "$6")
  local got; got=$(curl "${args[@]}")
  if [ "$got" = "$2" ]; then echo "PASS $1 ($got)"; else echo "FAIL $1: want $2 got $got"; cat /tmp/smoke.out; echo; fi
}
is() { if [ "$2" = "$3" ]; then echo "  ok   $1 = $2"; else echo "  FAIL $1: want $3 got $2"; fi; }

echo "health"; curl -s $BASE/health; echo
req config 200 GET /config
is premises "$(j '.premises|length')" 1

req proximity-on 200 POST /proximity/check "" "{\"location\":$ON}"
is zone "$(j .zone)" on_premises; is unlocked "$(j .biometrics_unlocked)" true
req proximity-off 200 POST /proximity/check "" "{\"location\":$OFF}"
is zone "$(j .zone)" off_premises; is unlocked "$(j .biometrics_unlocked)" false
req proximity-mock 403 POST /proximity/check "" "{\"location\":$MOCK}"

login_body() { jq -n --arg t "dev:$1" --arg d "$DEV" --arg k "$PUB" --argjson l "$2" \
  '{id_token:$t,device:{device_id:$d,platform:"android",name:"Smoke phone",biometric_public_key:$k},location:$l}'; }
req google-off-campus 403 POST /auth/google "" "$(login_body $EMAIL "$OFF")"
req google-login 200 POST /auth/google "" "$(login_body $EMAIL "$ON")"
TOKEN=$(j .access_token); REFRESH=$(j .refresh_token)
is initials "$(j .user.initials)" "$(echo ${EMAIL%%@*} | cut -c1 | tr a-z A-Z)$(echo ${EMAIL%%@*} | cut -c2 | tr a-z A-Z)"
req device-bound 409 POST /auth/google "" "$(login_body other$RUN@$DOMAIN "$ON")"
req bad-token 401 GET /me "bogus"

req me 200 GET /me "$TOKEN"; is biometric_enrolled "$(j .device.biometric_enrolled)" true
req settings 200 PATCH /me/settings "$TOKEN" '{"theme":"light","background_polling":false}'
is theme "$(j .theme)" light
req settings-bad 400 PATCH /me/settings "$TOKEN" '{"theme":"pink"}'
req me-after 200 GET /me "$TOKEN"; is theme "$(j .settings.theme)" light

req today-empty 200 GET /attendance/today "$TOKEN"; is checked_in "$(j .checked_in)" false
req hb-before-bio 200 POST /attendance/heartbeat "$TOKEN" "{\"location\":$ON,\"source\":\"auto\"}"
is state "$(j .radar.state)" near_premises; is check_in_required "$(j .radar.check_in_required)" true

req att-challenge 200 POST /attendance/biometric/challenge "$TOKEN"
CID=$(j .challenge_id); NONCE=$(j .nonce)
req hb-bad-sig 401 POST /attendance/heartbeat "$TOKEN" \
  "{\"location\":$ON,\"source\":\"manual\",\"biometric\":{\"challenge_id\":\"$CID\",\"signature\":\"AAAA\"}}"
req att-challenge2 200 POST /attendance/biometric/challenge "$TOKEN"
CID=$(j .challenge_id); NONCE=$(j .nonce)
req hb-check-in 200 POST /attendance/heartbeat "$TOKEN" \
  "{\"location\":$ON,\"source\":\"manual\",\"biometric\":{\"challenge_id\":\"$CID\",\"signature\":\"$(sign $NONCE)\"}}"
is state "$(j .radar.state)" checked_in; is verified "$(j .today.verified)" true
is event "$(j '.today.recent_events[0].title')" "First Check-In (Time In)"
FIRST=$(j .today.first_seen_at)
req replay 401 POST /attendance/heartbeat "$TOKEN" \
  "{\"location\":$ON,\"biometric\":{\"challenge_id\":\"$CID\",\"signature\":\"$(sign $NONCE)\"}}"

sleep 2
req hb-auto 200 POST /attendance/heartbeat "$TOKEN" "{\"location\":$ON,\"source\":\"auto\"}"
is first_seen_locked "$(j .today.first_seen_at)" "$FIRST"
echo "  last_seen $(j .today.last_seen_at)  active_seconds $(j .today.active_seconds)"
req hb-off 200 POST /attendance/heartbeat "$TOKEN" "{\"location\":$OFF,\"source\":\"auto\"}"
is state "$(j .radar.state)" off_premises
req hb-mock 403 POST /attendance/heartbeat "$TOKEN" "{\"location\":$MOCK}"

req today 200 GET /attendance/today "$TOKEN"; is checked_in "$(j .checked_in)" true
req months 200 GET /attendance/months "$TOKEN"; echo "  $(j -c .months 2>/dev/null || j .months)"
req history 200 GET /attendance/history "$TOKEN"
is present_days "$(j .summary.present_days)" 1; is verified_percent "$(j .summary.verified_percent)" 100
is today_status "$(j '.days[0].status')" active
req history-bad 400 GET "/attendance/history?month=nope" "$TOKEN"
req devices 200 GET /me/devices "$TOKEN"
req key-put 200 PUT /me/device/biometric-key "$TOKEN" "{\"public_key\":\"$PUB\"}"

req refresh 200 POST /auth/refresh "" "{\"refresh_token\":\"$REFRESH\"}"
req refresh-reuse 401 POST /auth/refresh "" "{\"refresh_token\":\"$REFRESH\"}"

req bio-chal-off 403 POST /auth/biometric/challenge "" "{\"device_id\":\"$DEV\",\"location\":$OFF}"
req bio-chal 200 POST /auth/biometric/challenge "" "{\"device_id\":\"$DEV\",\"location\":$ON}"
CID=$(j .challenge_id); NONCE=$(j .nonce)
req bio-login 200 POST /auth/biometric/login "" \
  "{\"device_id\":\"$DEV\",\"challenge_id\":\"$CID\",\"signature\":\"$(sign $NONCE)\",\"location\":$ON}"
T2=$(j .access_token)
req logout 204 POST /auth/logout "$T2"
req after-logout 401 GET /me "$T2"
echo done
