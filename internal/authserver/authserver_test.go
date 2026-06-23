package authserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer() *AuthServer {
	return New(0)
}

func TestAuthenticate(t *testing.T) {
	as := newTestServer()

	body := `{"username":"Steve","password":"any"}`
	req := httptest.NewRequest("POST", "/authserver/authenticate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	as.handleAuthenticate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp AuthenticateResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if resp.AccessToken == "" {
		t.Error("accessToken should not be empty")
	}
	if resp.ClientToken == "" {
		t.Error("clientToken should not be empty")
	}
	if resp.SelectedProfile == nil {
		t.Fatal("selectedProfile should not be nil")
	}
	if resp.SelectedProfile.Name != "Steve" {
		t.Errorf("expected Steve, got %s", resp.SelectedProfile.Name)
	}
	if len(resp.AvailableProfiles) != 1 {
		t.Errorf("expected 1 profile, got %d", len(resp.AvailableProfiles))
	}
}

func TestAuthenticate_ConsistentUUID(t *testing.T) {
	as := newTestServer()

	login := func(name string) string {
		body := `{"username":"` + name + `","password":"x"}`
		req := httptest.NewRequest("POST", "/authserver/authenticate", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		as.handleAuthenticate(w, req)
		var resp AuthenticateResp
		json.Unmarshal(w.Body.Bytes(), &resp)
		return resp.SelectedProfile.ID
	}

	u1 := login("Alice")
	u2 := login("Alice")
	if u1 != u2 {
		t.Errorf("same username should get same UUID: %s vs %s", u1, u2)
	}
}

func TestAuthenticate_MultipleUsers(t *testing.T) {
	as := newTestServer()

	body := `{"username":"Bob","password":"x"}`
	req := httptest.NewRequest("POST", "/authserver/authenticate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	as.handleAuthenticate(w, req)

	var resp AuthenticateResp
	json.Unmarshal(w.Body.Bytes(), &resp)
	token := resp.AccessToken
	clientToken := resp.ClientToken

	refreshBody := `{"accessToken":"` + token + `","clientToken":"` + clientToken + `"}`
	req2 := httptest.NewRequest("POST", "/authserver/refresh", bytes.NewBufferString(refreshBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	as.handleRefresh(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("refresh expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var refreshResp RefreshResp
	json.Unmarshal(w2.Body.Bytes(), &refreshResp)
	if refreshResp.AccessToken == token {
		t.Error("refresh should return a new token")
	}
}

func TestValidate_Valid(t *testing.T) {
	as := newTestServer()

	body := `{"username":"Test","password":"x"}`
	req := httptest.NewRequest("POST", "/authserver/authenticate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	as.handleAuthenticate(w, req)

	var auth AuthenticateResp
	json.Unmarshal(w.Body.Bytes(), &auth)

	valBody := `{"accessToken":"` + auth.AccessToken + `","clientToken":"` + auth.ClientToken + `"}`
	req2 := httptest.NewRequest("POST", "/authserver/validate", bytes.NewBufferString(valBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	as.handleValidate(w2, req2)

	if w2.Code != http.StatusNoContent {
		t.Errorf("validate expected 204, got %d", w2.Code)
	}
}

func TestValidate_Invalid(t *testing.T) {
	as := newTestServer()

	body := `{"accessToken":"bad","clientToken":"bad"}`
	req := httptest.NewRequest("POST", "/authserver/validate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	as.handleValidate(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestInvalidate(t *testing.T) {
	as := newTestServer()

	body := `{"username":"Temp","password":"x"}`
	req := httptest.NewRequest("POST", "/authserver/authenticate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	as.handleAuthenticate(w, req)

	var auth AuthenticateResp
	json.Unmarshal(w.Body.Bytes(), &auth)

	invBody := `{"accessToken":"` + auth.AccessToken + `","clientToken":"` + auth.ClientToken + `"}`
	req2 := httptest.NewRequest("POST", "/authserver/invalidate", bytes.NewBufferString(invBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	as.handleInvalidate(w2, req2)

	if w2.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w2.Code)
	}

	valBody := `{"accessToken":"` + auth.AccessToken + `","clientToken":"` + auth.ClientToken + `"}`
	req3 := httptest.NewRequest("POST", "/authserver/validate", bytes.NewBufferString(valBody))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	as.handleValidate(w3, req3)

	if w3.Code != http.StatusForbidden {
		t.Errorf("expected 403 after invalidate, got %d", w3.Code)
	}
}

func TestJoinAndHasJoined(t *testing.T) {
	as := newTestServer()

	body := `{"username":"Player","password":"x"}`
	req := httptest.NewRequest("POST", "/authserver/authenticate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	as.handleAuthenticate(w, req)

	var auth AuthenticateResp
	json.Unmarshal(w.Body.Bytes(), &auth)
	uuid := auth.SelectedProfile.ID

	joinBody := `{"accessToken":"` + auth.AccessToken + `","selectedProfile":"` + uuid + `","serverId":"test-server-1"}`
	req2 := httptest.NewRequest("POST", "/sessionserver/session/minecraft/join", bytes.NewBufferString(joinBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	as.handleJoin(w2, req2)

	if w2.Code != http.StatusNoContent {
		t.Fatalf("join expected 204, got %d: %s", w2.Code, w2.Body.String())
	}

	req3 := httptest.NewRequest("GET", "/sessionserver/session/minecraft/hasJoined?username=Player&serverId=test-server-1", nil)
	w3 := httptest.NewRecorder()
	as.handleHasJoined(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("hasJoined expected 200, got %d: %s", w3.Code, w3.Body.String())
	}

	var hasJoined HasJoinedResp
	json.Unmarshal(w3.Body.Bytes(), &hasJoined)
	if hasJoined.Name != "Player" {
		t.Errorf("expected Player, got %s", hasJoined.Name)
	}
	if len(hasJoined.Properties) == 0 {
		t.Error("expected texture properties")
	}
}

func TestHasJoined_NotJoined(t *testing.T) {
	as := newTestServer()

	req := httptest.NewRequest("GET", "/sessionserver/session/minecraft/hasJoined?username=Ghost&serverId=no-such-server", nil)
	w := httptest.NewRecorder()
	as.handleHasJoined(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for not joined, got %d", w.Code)
	}
}

func TestProfile(t *testing.T) {
	as := newTestServer()

	body := `{"username":"ProfileTest","password":"x"}`
	req := httptest.NewRequest("POST", "/authserver/authenticate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	as.handleAuthenticate(w, req)

	var auth AuthenticateResp
	json.Unmarshal(w.Body.Bytes(), &auth)
	uuid := auth.SelectedProfile.ID

	req2 := httptest.NewRequest("GET", "/sessionserver/session/minecraft/profile/"+uuid, nil)
	w2 := httptest.NewRecorder()
	as.handleProfile(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("profile expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var profile ProfileResp
	json.Unmarshal(w2.Body.Bytes(), &profile)
	if profile.Name != "ProfileTest" {
		t.Errorf("expected ProfileTest, got %s", profile.Name)
	}
	if len(profile.Properties) == 0 {
		t.Error("expected texture properties")
	}
}

func TestProfile_NotFound(t *testing.T) {
	as := newTestServer()

	req := httptest.NewRequest("GET", "/sessionserver/session/minecraft/profile/no-such-uuid", nil)
	w := httptest.NewRecorder()
	as.handleProfile(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRoot(t *testing.T) {
	as := newTestServer()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	as.handleRoot(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	meta, ok := resp["meta"].(map[string]interface{})
	if !ok {
		t.Fatal("meta field missing")
	}
	if meta["serverName"] != "Offline Launcher Auth Server" {
		t.Errorf("unexpected server name: %v", meta["serverName"])
	}
}

func TestStore_RegisterAndToken(t *testing.T) {
	s := NewStore()
	acc := s.Register("Alice", "uuid-alice", "ct-1")
	if acc == nil {
		t.Fatal("register returned nil")
	}
	if acc.AccessToken == "" {
		t.Error("accessToken should not be empty")
	}

	owner := s.TokenOwner(acc.AccessToken)
	if owner == nil || owner.UUID != "uuid-alice" {
		t.Error("TokenOwner failed")
	}
}

func TestStore_Signout(t *testing.T) {
	s := NewStore()
	s.Register("Bob", "uuid-bob", "ct-2")
	if !s.Signout("Bob") {
		t.Error("signout should succeed")
	}
	if s.Signout("Bob") {
		t.Error("second signout should fail")
	}
}

func TestTextureProvider_CustomSkin(t *testing.T) {
	as := newTestServer()
	as.SetTextureProvider(func(uuid, username string) (string, bool) {
		if username == "SkinUser" {
			return base64.StdEncoding.EncodeToString([]byte(`{"custom":true}`)), true
		}
		return "", false
	})

	body := `{"username":"SkinUser","password":"x"}`
	req := httptest.NewRequest("POST", "/authserver/authenticate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	as.handleAuthenticate(w, req)

	var auth AuthenticateResp
	json.Unmarshal(w.Body.Bytes(), &auth)
	uuid := auth.SelectedProfile.ID

	req2 := httptest.NewRequest("GET", "/sessionserver/session/minecraft/profile/"+uuid, nil)
	w2 := httptest.NewRecorder()
	as.handleProfile(w2, req2)

	var profile ProfileResp
	json.Unmarshal(w2.Body.Bytes(), &profile)
	if len(profile.Properties) == 0 {
		t.Fatal("expected properties")
	}

	decoded, _ := base64.StdEncoding.DecodeString(profile.Properties[0].Value)
	if !strings.Contains(string(decoded), `"custom":true`) {
		t.Errorf("expected custom texture, got %s", string(decoded))
	}
}

func TestTextureProvider_FallbackToDefault(t *testing.T) {
	as := newTestServer()
	// textureProvider is nil -> falls back to generateDefaultSkinPayload

	body := `{"username":"DefaultUser","password":"x"}`
	req := httptest.NewRequest("POST", "/authserver/authenticate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	as.handleAuthenticate(w, req)

	var auth AuthenticateResp
	json.Unmarshal(w.Body.Bytes(), &auth)
	uuid := auth.SelectedProfile.ID

	req2 := httptest.NewRequest("GET", "/sessionserver/session/minecraft/profile/"+uuid, nil)
	w2 := httptest.NewRecorder()
	as.handleProfile(w2, req2)

	var profile ProfileResp
	json.Unmarshal(w2.Body.Bytes(), &profile)
	if len(profile.Properties) == 0 {
		t.Fatal("expected properties")
	}

	decoded, _ := base64.StdEncoding.DecodeString(profile.Properties[0].Value)
	if !strings.Contains(string(decoded), "textures.minecraft.net") {
		t.Errorf("expected default texture URL, got %s", string(decoded))
	}
}

func TestTextureProvider_HasJoinedCustom(t *testing.T) {
	as := newTestServer()
	as.SetTextureProvider(func(uuid, username string) (string, bool) {
		return base64.StdEncoding.EncodeToString([]byte(`{"hasJoined":true}`)), true
	})

	body := `{"username":"JoinUser","password":"x"}`
	req := httptest.NewRequest("POST", "/authserver/authenticate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	as.handleAuthenticate(w, req)

	var auth AuthenticateResp
	json.Unmarshal(w.Body.Bytes(), &auth)
	uuid := auth.SelectedProfile.ID

	joinBody := `{"accessToken":"` + auth.AccessToken + `","selectedProfile":"` + uuid + `","serverId":"s1"}`
	req2 := httptest.NewRequest("POST", "/sessionserver/session/minecraft/join", bytes.NewBufferString(joinBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	as.handleJoin(w2, req2)

	req3 := httptest.NewRequest("GET", "/sessionserver/session/minecraft/hasJoined?username=JoinUser&serverId=s1", nil)
	w3 := httptest.NewRecorder()
	as.handleHasJoined(w3, req3)

	var hasJoined HasJoinedResp
	json.Unmarshal(w3.Body.Bytes(), &hasJoined)
	if len(hasJoined.Properties) == 0 {
		t.Fatal("expected texture properties")
	}

	decoded, _ := base64.StdEncoding.DecodeString(hasJoined.Properties[0].Value)
	if !strings.Contains(string(decoded), `"hasJoined":true`) {
		t.Errorf("expected custom hasJoined texture, got %s", string(decoded))
	}
}

func TestSetSkinsDir(t *testing.T) {
	as := newTestServer()
	as.SetSkinsDir("/tmp/test-skins")
	// just ensure no panic/error
	_ = as
}

func TestJSONStructure(t *testing.T) {
	as := newTestServer()

	body := `{"username":"StructTest","password":"x"}`
	req := httptest.NewRequest("POST", "/authserver/authenticate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	as.handleAuthenticate(w, req)

	var resp AuthenticateResp
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.User == nil {
		t.Fatal("user should be present")
	}
	if !resp.User.UsernameKnown {
		t.Error("usernameKnown should be true")
	}
	if resp.User.Username != "StructTest" {
		t.Errorf("expected StructTest, got %s", resp.User.Username)
	}
}
