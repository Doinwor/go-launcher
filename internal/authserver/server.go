package authserver

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
)

type TextureProvider func(uuid, username string) (payload string, ok bool)

type AuthServer struct {
	mu             sync.RWMutex
	server         *http.Server
	store          *Store
	running        bool
	port           int
	textureFn      TextureProvider
	skinsDir       string
}

func New(port int) *AuthServer {
	return &AuthServer{
		store: NewStore(),
		port:  port,
	}
}

func (as *AuthServer) SetTextureProvider(fn TextureProvider) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.textureFn = fn
}

func (as *AuthServer) SetSkinsDir(dir string) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.skinsDir = dir
}

func (as *AuthServer) Start() error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if as.running {
		return fmt.Errorf("auth server already running on port %d", as.port)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/authserver/authenticate", as.handleAuthenticate)
	mux.HandleFunc("/authserver/refresh", as.handleRefresh)
	mux.HandleFunc("/authserver/validate", as.handleValidate)
	mux.HandleFunc("/authserver/invalidate", as.handleInvalidate)
	mux.HandleFunc("/authserver/signout", as.handleSignout)
	mux.HandleFunc("/sessionserver/session/minecraft/join", as.handleJoin)
	mux.HandleFunc("/sessionserver/session/minecraft/hasJoined", as.handleHasJoined)
	mux.HandleFunc("/sessionserver/session/minecraft/profile/", as.handleProfile)
	mux.HandleFunc("/api/register", as.handleRegister)
	mux.HandleFunc("/", as.handleRoot)

	if as.skinsDir != "" {
		mux.Handle("/skin/", http.StripPrefix("/skin/", http.FileServer(http.Dir(as.skinsDir))))
	}

	as.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", as.port),
		Handler: mux,
	}

	as.running = true

	go func() {
		log.Printf("[authserver] listening on http://localhost:%d", as.port)
		if err := as.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[authserver] error: %v", err)
		}
		as.mu.Lock()
		as.running = false
		as.mu.Unlock()
	}()

	return nil
}

func (as *AuthServer) Stop() error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if !as.running {
		return fmt.Errorf("auth server is not running")
	}

	if as.server != nil {
		if err := as.server.Close(); err != nil {
			return err
		}
	}

	as.running = false
	return nil
}

func (as *AuthServer) IsRunning() bool {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.running
}

func (as *AuthServer) Port() int {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.port
}

func (as *AuthServer) Store() *Store {
	return as.store
}

func (as *AuthServer) texturePayload(uuid, username string) string {
	as.mu.RLock()
	fn := as.textureFn
	as.mu.RUnlock()

	if fn != nil {
		if payload, ok := fn(uuid, username); ok {
			return payload
		}
	}

	return generateDefaultSkinPayload(uuid, username)
}

func (as *AuthServer) injectUser(acc *storedAccount, clientToken string) *AuthenticateResp {
	return &AuthenticateResp{
		AccessToken:       acc.AccessToken,
		ClientToken:       clientToken,
		AvailableProfiles: acc.Profiles,
		SelectedProfile:   &acc.Profiles[0],
		User: &UserResp{
			ID:            acc.UUID,
			Username:      acc.Username,
			UsernameKnown: true,
		},
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, errMsg string) {
	writeJSON(w, status, ErrorResp{
		Error:        http.StatusText(status),
		ErrorMessage: errMsg,
	})
}

func (as *AuthServer) handleAuthenticate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req AuthenticateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	clientToken := req.ClientToken
	if clientToken == "" {
		b := make([]byte, 16)
		rand.Read(b)
		clientToken = fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	}

	acc, ct := as.store.Authenticate(req.Username, clientToken)
	if acc == nil {
		uuid := generateUUIDv3(req.Username)
		acc = as.store.Register(req.Username, uuid, clientToken)
		ct = clientToken
	}

	writeJSON(w, http.StatusOK, as.injectUser(acc, ct))
}

func (as *AuthServer) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req RefreshReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	acc, newToken, ok := as.store.Refresh(req.AccessToken, req.ClientToken)
	if !ok {
		writeError(w, http.StatusForbidden, "Invalid token")
		return
	}

	resp := RefreshResp{
		AccessToken:     newToken,
		ClientToken:     acc.ClientToken,
		SelectedProfile: &acc.Profiles[0],
	}

	writeJSON(w, http.StatusOK, resp)
}

func (as *AuthServer) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req ValidateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if as.store.Validate(req.AccessToken, req.ClientToken) {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeError(w, http.StatusForbidden, "Invalid token")
}

func (as *AuthServer) handleInvalidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req InvalidReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	as.store.Invalidate(req.AccessToken, req.ClientToken)
	w.WriteHeader(http.StatusNoContent)
}

func (as *AuthServer) handleSignout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req SignoutReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	as.store.Signout(req.Username)
	w.WriteHeader(http.StatusNoContent)
}

func (as *AuthServer) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req JoinReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if as.store.Join(req.AccessToken, req.SelectedProfile, req.ServerID) {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeError(w, http.StatusForbidden, "Invalid token or profile")
}

func (as *AuthServer) handleHasJoined(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	username := r.URL.Query().Get("username")
	serverID := r.URL.Query().Get("serverId")

	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	acc := as.store.HasJoined(username, serverID)
	if acc == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := HasJoinedResp{
		UUID: acc.UUID,
		Name: acc.Username,
		Properties: []Property{
			{
				Name:  "textures",
				Value: as.texturePayload(acc.UUID, acc.Username),
			},
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

func (as *AuthServer) handleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/sessionserver/session/minecraft/profile/")
	uuid := strings.Split(path, "?")[0]
	unsigned := r.URL.Query().Get("unsigned")

	acc := as.store.Profile(uuid)
	if acc == nil {
		writeError(w, http.StatusNotFound, "Profile not found")
		return
	}

	resp := ProfileResp{
		ID:   acc.UUID,
		Name: acc.Username,
	}

	if unsigned != "true" {
		resp.Properties = []Property{
			{
				Name:  "textures",
				Value: as.texturePayload(acc.UUID, acc.Username),
			},
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (as *AuthServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	uuid := generateUUIDv3(req.Username)
	clientToken := randomHex(16)
	as.store.Register(req.Username, uuid, clientToken)
	as.store.Authenticate(req.Username, clientToken)

	writeJSON(w, http.StatusOK, map[string]string{
		"uuid": uuid,
		"username": req.Username,
	})
}

func (as *AuthServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"meta": map[string]interface{}{
			"serverName":    "Offline Launcher Auth Server",
			"implementationName": "offline-launcher",
			"implementationVersion": "1.0.0",
		},
		"skinDomains": []string{"localhost", "*.minecraft.net"},
		"signaturePublickey": nil,
	})
}

func generateUUIDv3(name string) string {
	h := md5Hash(name)
	h[6] = (h[6] & 0x0f) | 0x30
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

func md5Hash(s string) []byte {
	d := make([]byte, 16)
	for i := 0; i < 16; i++ {
		d[i] = byte((i*7 + len(s)*3) & 0xff)
	}
	return d
}

func generateDefaultSkinPayload(uuid, username string) string {
	textures := SkinTextures{
		Timestamp:    1,
		ProfileID:    strings.ReplaceAll(uuid, "-", ""),
		ProfileName:  username,
		Textures: TexturesMap{
			SKIN: &SkinInfo{
				URL: "http://textures.minecraft.net/texture/1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a",
			},
		},
	}

	data, _ := json.Marshal(textures)
	return base64.StdEncoding.EncodeToString(data)
}
