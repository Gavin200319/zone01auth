package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"z01auth/internal/attendance"
	"z01auth/internal/auth"
	"z01auth/internal/config"
	"z01auth/internal/store"
)

const challengeTTL = 2 * time.Minute

type Handler struct {
	Cfg      *config.Config
	Store    *store.Store
	Att      *attendance.Service
	Tokens   *auth.Issuer
	Verifier auth.Verifier
}

func errProximity() *APIError {
	return newErr(http.StatusForbidden, "PROXIMITY_REQUIRED", "you must be on campus (Wi-Fi or GPS) to do this")
}

func (h *Handler) gate(l locationIn) error {
	rd, err := readLocation(l)
	if err != nil {
		return err
	}
	if !h.Att.Evaluate(rd).Unlocked {
		return errProximity()
	}
	return nil
}

func uid(c *fiber.Ctx) string { return c.Locals("uid").(string) }

func did(c *fiber.Ctx) string { return c.Locals("did").(string) }

func sid(c *fiber.Ctx) string { return c.Locals("sid").(string) }

func ctxOf(c *fiber.Ctx) context.Context { return c.UserContext() }

func (h *Handler) RequireAuth(c *fiber.Ctx) error {
	hdr := c.Get("Authorization")
	token, ok := strings.CutPrefix(hdr, "Bearer ")
	if !ok || token == "" {
		return unauthorized("missing bearer token")
	}
	claims, err := h.Tokens.Parse(token)
	if err != nil {
		return unauthorized("invalid or expired token")
	}
	active, err := h.Store.SessionActive(ctxOf(c), claims.SessionID)
	if err != nil {
		return err
	}
	if !active {
		return unauthorized("session has ended")
	}
	c.Locals("uid", claims.Subject)
	c.Locals("did", claims.DeviceID)
	c.Locals("sid", claims.SessionID)
	return c.Next()
}

func (h *Handler) Health(c *fiber.Ctx) error {
	if err := h.Store.Ping(ctxOf(c)); err != nil {
		return newErr(http.StatusServiceUnavailable, "UNAVAILABLE", "database unreachable")
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func (h *Handler) HealthLive(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "alive",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handler) HealthReady(c *fiber.Ctx) error {
	if err := h.Store.Ping(ctxOf(c)); err != nil {
		return newErr(http.StatusServiceUnavailable, "UNAVAILABLE", "database unreachable")
	}
	return c.JSON(fiber.Map{
		"status":   "ready",
		"database": "connected",
	})
}

func (h *Handler) Config(c *fiber.Ctx) error {
	list := make([]fiber.Map, 0, len(h.Att.Premises))
	for _, p := range h.Att.Premises {
		list = append(list, fiber.Map{
			"id":                p.ID,
			"name":              p.Name,
			"ssid":              p.SSID,
			"lat":               p.Lat,
			"lng":               p.Lng,
			"geofence_radius_m": p.GeofenceM,
			"far_radius_m":      p.FarM,
		})
	}
	return c.JSON(fiber.Map{
		"premises":                   list,
		"timezone":                   h.Cfg.Location.String(),
		"heartbeat_interval_seconds": h.Cfg.HeartbeatSeconds,
		"require_biometric_time_in":  h.Cfg.RequireBiometricTimeIn,
		"max_gps_accuracy_m":         h.Cfg.MaxGPSAccuracyM,
	})
}

func (h *Handler) ProximityCheck(c *fiber.Ctx) error {
	var in struct {
		Location locationIn `json:"location"`
	}
	if err := parseBody(c, &in); err != nil {
		return err
	}
	rd, err := readLocation(in.Location)
	if err != nil {
		return err
	}
	return c.JSON(h.Att.Check(rd))
}

func (h *Handler) startSession(ctx context.Context, u store.User, d store.Device) (*authResponse, error) {
	raw, hash := auth.NewRefreshToken()
	sessionID, err := h.Store.CreateSession(ctx, u.ID, d.ID, hash, time.Now().Add(h.Cfg.RefreshTTL))
	if err != nil {
		return nil, err
	}
	access, err := h.Tokens.Issue(u.ID, sessionID, d.ID)
	if err != nil {
		return nil, err
	}
	settings, err := h.Store.GetSettings(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &authResponse{
		AccessToken:  access,
		RefreshToken: raw,
		TokenType:    "Bearer",
		ExpiresIn:    int(h.Tokens.TTL().Seconds()),
		User:         toUserView(u),
		Settings:     toSettingsView(settings),
		Device:       toDeviceView(d),
	}, nil
}

func (h *Handler) domainAllowed(email string) bool {
	if len(h.Cfg.AllowedEmailDomains) == 0 {
		return true
	}
	for _, d := range h.Cfg.AllowedEmailDomains {
		if strings.HasSuffix(email, "@"+d) {
			return true
		}
	}
	return false
}

func (h *Handler) GoogleLogin(c *fiber.Ctx) error {
	var in struct {
		IDToken  string     `json:"id_token"`
		Device   deviceIn   `json:"device"`
		Location locationIn `json:"location"`
	}
	if err := parseBody(c, &in); err != nil {
		return err
	}
	if in.IDToken == "" {
		return badRequest("id_token is required")
	}
	if err := validateDevice(in.Device); err != nil {
		return err
	}
	if in.Device.BiometricPublicKey != "" {
		if _, err := auth.ParsePublicKey(in.Device.BiometricPublicKey); err != nil {
			return badRequest("device.biometric_public_key: " + err.Error())
		}
	}
	if err := h.gate(in.Location); err != nil {
		return err
	}

	gu, err := h.Verifier.Verify(ctxOf(c), in.IDToken)
	if err != nil {
		return unauthorized("google sign-in could not be verified")
	}
	if !h.domainAllowed(gu.Email) {
		return newErr(http.StatusForbidden, "EMAIL_DOMAIN_NOT_ALLOWED", "this email domain is not allowed")
	}

	u, err := h.Store.UpsertUser(ctxOf(c), gu.Sub, gu.Email, gu.Name, gu.Picture)
	if err != nil {
		return err
	}
	d, err := h.Store.BindDevice(ctxOf(c), u.ID, in.Device.DeviceID, in.Device.Platform, in.Device.Name, in.Device.BiometricPublicKey)
	if errors.Is(err, store.ErrDeviceBound) {
		return newErr(http.StatusConflict, "DEVICE_BOUND", "this device is bound to another account")
	}
	if err != nil {
		return err
	}
	resp, err := h.startSession(ctxOf(c), u, d)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

func (h *Handler) BiometricLoginChallenge(c *fiber.Ctx) error {
	var in struct {
		DeviceID string     `json:"device_id"`
		Location locationIn `json:"location"`
	}
	if err := parseBody(c, &in); err != nil {
		return err
	}
	if in.DeviceID == "" {
		return badRequest("device_id is required")
	}
	if err := h.gate(in.Location); err != nil {
		return err
	}
	d, err := h.Store.FindBiometricDevice(ctxOf(c), in.DeviceID)
	if errors.Is(err, store.ErrNotFound) {
		return newErr(http.StatusNotFound, "BIOMETRIC_NOT_ENROLLED", "no biometric key is enrolled for this device; sign in with Google first")
	}
	if err != nil {
		return err
	}
	return h.issueChallenge(c, d, "login")
}

func (h *Handler) issueChallenge(c *fiber.Ctx, d store.Device, purpose string) error {
	nonce := auth.NewNonce()
	id, err := h.Store.CreateChallenge(ctxOf(c), d.UserID, d.ID, purpose, nonce, time.Now().Add(challengeTTL))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"challenge_id": id,
		"nonce":        nonce,
		"expires_in":   int(challengeTTL.Seconds()),
	})
}

func (h *Handler) verifyChallenge(ctx context.Context, d store.Device, purpose, challengeID, signature string) error {
	if challengeID == "" || signature == "" {
		return newErr(http.StatusUnauthorized, "BIOMETRIC_FAILED", "challenge_id and signature are required")
	}
	nonce, err := h.Store.ConsumeChallenge(ctx, challengeID, d.ID, purpose)
	if errors.Is(err, store.ErrNotFound) {
		return newErr(http.StatusUnauthorized, "BIOMETRIC_FAILED", "challenge is invalid, used or expired")
	}
	if err != nil {
		return err
	}
	if err := auth.VerifySignature(d.BiometricKey, nonce, signature); err != nil {
		return newErr(http.StatusUnauthorized, "BIOMETRIC_FAILED", "biometric signature did not match")
	}
	return nil
}

func (h *Handler) BiometricLogin(c *fiber.Ctx) error {
	var in struct {
		DeviceID    string     `json:"device_id"`
		ChallengeID string     `json:"challenge_id"`
		Signature   string     `json:"signature"`
		Location    locationIn `json:"location"`
	}
	if err := parseBody(c, &in); err != nil {
		return err
	}
	if err := h.gate(in.Location); err != nil {
		return err
	}
	d, err := h.Store.FindBiometricDevice(ctxOf(c), in.DeviceID)
	if errors.Is(err, store.ErrNotFound) {
		return newErr(http.StatusNotFound, "BIOMETRIC_NOT_ENROLLED", "no biometric key is enrolled for this device")
	}
	if err != nil {
		return err
	}
	if err := h.verifyChallenge(ctxOf(c), d, "login", in.ChallengeID, in.Signature); err != nil {
		return err
	}
	u, err := h.Store.GetUser(ctxOf(c), d.UserID)
	if err != nil {
		return err
	}
	resp, err := h.startSession(ctxOf(c), u, d)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

func (h *Handler) Refresh(c *fiber.Ctx) error {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := parseBody(c, &in); err != nil {
		return err
	}
	if in.RefreshToken == "" {
		return badRequest("refresh_token is required")
	}
	raw, hash := auth.NewRefreshToken()
	ss, err := h.Store.RotateSession(ctxOf(c), auth.HashToken(in.RefreshToken), hash, time.Now().Add(h.Cfg.RefreshTTL))
	if errors.Is(err, store.ErrNotFound) {
		return unauthorized("refresh token is invalid or expired")
	}
	if err != nil {
		return err
	}
	access, err := h.Tokens.Issue(ss.UserID, ss.ID, ss.DeviceID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"access_token":  access,
		"refresh_token": raw,
		"token_type":    "Bearer",
		"expires_in":    int(h.Tokens.TTL().Seconds()),
	})
}

func (h *Handler) Logout(c *fiber.Ctx) error {
	if err := h.Store.RevokeSession(ctxOf(c), sid(c)); err != nil {
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

func (h *Handler) Me(c *fiber.Ctx) error {
	u, err := h.Store.GetUser(ctxOf(c), uid(c))
	if err != nil {
		return err
	}
	st, err := h.Store.GetSettings(ctxOf(c), u.ID)
	if err != nil {
		return err
	}
	d, err := h.Store.GetDevice(ctxOf(c), did(c))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"user":     toUserView(u),
		"settings": toSettingsView(st),
		"device":   toDeviceView(d),
	})
}

func (h *Handler) UpdateSettings(c *fiber.Ctx) error {
	var in struct {
		Theme             *string `json:"theme"`
		BackgroundPolling *bool   `json:"background_polling"`
	}
	if err := parseBody(c, &in); err != nil {
		return err
	}
	if in.Theme != nil && *in.Theme != "dark" && *in.Theme != "light" {
		return badRequest("theme must be dark or light")
	}
	st, err := h.Store.UpdateSettings(ctxOf(c), uid(c), in.Theme, in.BackgroundPolling)
	if err != nil {
		return err
	}
	return c.JSON(toSettingsView(st))
}

func (h *Handler) SetBiometricKey(c *fiber.Ctx) error {
	var in struct {
		PublicKey string `json:"public_key"`
	}
	if err := parseBody(c, &in); err != nil {
		return err
	}
	if _, err := auth.ParsePublicKey(in.PublicKey); err != nil {
		return badRequest("public_key: " + err.Error())
	}
	if err := h.Store.SetBiometricKey(ctxOf(c), did(c), in.PublicKey); err != nil {
		return err
	}
	d, err := h.Store.GetDevice(ctxOf(c), did(c))
	if err != nil {
		return err
	}
	return c.JSON(toDeviceView(d))
}

func (h *Handler) ListDevices(c *fiber.Ctx) error {
	list, err := h.Store.ListDevices(ctxOf(c), uid(c))
	if err != nil {
		return err
	}
	out := make([]deviceView, 0, len(list))
	for _, d := range list {
		out = append(out, toDeviceView(d))
	}
	return c.JSON(fiber.Map{"devices": out, "current_device_id": did(c)})
}

func (h *Handler) DeleteDevice(c *fiber.Ctx) error {
	id := c.Params("id")
	if !store.ValidID(id) {
		return badRequest("invalid device id")
	}
	ok, err := h.Store.DeleteDevice(ctxOf(c), uid(c), id)
	if err != nil {
		return err
	}
	if !ok {
		return newErr(http.StatusNotFound, "NOT_FOUND", "device not found")
	}
	return c.SendStatus(http.StatusNoContent)
}

func (h *Handler) AttendanceToday(c *fiber.Ctx) error {
	v, err := h.Att.Today(ctxOf(c), uid(c))
	if err != nil {
		return err
	}
	return c.JSON(v)
}

func (h *Handler) AttendanceChallenge(c *fiber.Ctx) error {
	d, err := h.Store.GetDevice(ctxOf(c), did(c))
	if err != nil {
		return err
	}
	if d.BiometricKey == "" {
		return newErr(http.StatusConflict, "BIOMETRIC_NOT_ENROLLED", "enroll a biometric key for this device first")
	}
	return h.issueChallenge(c, d, "checkin")
}

func (h *Handler) Heartbeat(c *fiber.Ctx) error {
	var in struct {
		Location  locationIn `json:"location"`
		Source    string     `json:"source"`
		Biometric *struct {
			ChallengeID string `json:"challenge_id"`
			Signature   string `json:"signature"`
		} `json:"biometric"`
	}
	if err := parseBody(c, &in); err != nil {
		return err
	}
	if in.Source != "" && in.Source != "auto" && in.Source != "manual" {
		return badRequest("source must be auto or manual")
	}
	rd, err := readLocation(in.Location)
	if err != nil {
		return err
	}

	bioOK := false
	if in.Biometric != nil && h.Att.Evaluate(rd).Unlocked {
		d, err := h.Store.GetDevice(ctxOf(c), did(c))
		if err != nil {
			return err
		}
		if d.BiometricKey == "" {
			return newErr(http.StatusConflict, "BIOMETRIC_NOT_ENROLLED", "enroll a biometric key for this device first")
		}
		if err := h.verifyChallenge(ctxOf(c), d, "checkin", in.Biometric.ChallengeID, in.Biometric.Signature); err != nil {
			return err
		}
		bioOK = true
	}

	res, err := h.Att.Heartbeat(ctxOf(c), attendance.HeartbeatInput{
		UserID:      uid(c),
		DeviceID:    did(c),
		Reading:     rd,
		Manual:      in.Source == "manual" || bioOK,
		BiometricOK: bioOK,
	})
	if err != nil {
		return err
	}
	return c.JSON(res)
}

func (h *Handler) History(c *fiber.Ctx) error {
	v, err := h.Att.History(ctxOf(c), uid(c), c.Query("month"))
	if errors.Is(err, attendance.ErrInvalidMonth) {
		return badRequest(err.Error())
	}
	if err != nil {
		return err
	}
	return c.JSON(v)
}

func (h *Handler) Months(c *fiber.Ctx) error {
	m, err := h.Att.Months(ctxOf(c), uid(c))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"months": m})
}
