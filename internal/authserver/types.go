package authserver

type AuthenticateReq struct {
	Agent       *Agent   `json:"agent"`
	Username    string   `json:"username"`
	Password    string   `json:"password"`
	ClientToken string   `json:"clientToken"`
	RequestUser bool     `json:"requestUser"`
}

type Agent struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

type AuthenticateResp struct {
	AccessToken       string             `json:"accessToken"`
	ClientToken       string             `json:"clientToken"`
	AvailableProfiles []ProfileResp      `json:"availableProfiles"`
	SelectedProfile   *ProfileResp       `json:"selectedProfile"`
	User              *UserResp          `json:"user,omitempty"`
}

type RefreshReq struct {
	AccessToken       string       `json:"accessToken"`
	ClientToken       string       `json:"clientToken"`
	RequestUser       bool         `json:"requestUser"`
	SelectedProfile   *ProfileResp `json:"selectedProfile,omitempty"`
}

type RefreshResp struct {
	AccessToken     string       `json:"accessToken"`
	ClientToken     string       `json:"clientToken"`
	SelectedProfile *ProfileResp `json:"selectedProfile"`
	User            *UserResp    `json:"user,omitempty"`
}

type ValidateReq struct {
	AccessToken string `json:"accessToken"`
	ClientToken string `json:"clientToken"`
}

type InvalidReq struct {
	AccessToken string `json:"accessToken"`
	ClientToken string `json:"clientToken"`
}

type SignoutReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type JoinReq struct {
	AccessToken     string `json:"accessToken"`
	SelectedProfile string `json:"selectedProfile"`
	ServerID        string `json:"serverId"`
}

type HasJoinedResp struct {
	UUID         string          `json:"id"`
	Name         string          `json:"name"`
	Properties   []Property      `json:"properties,omitempty"`
	ProfileProps []Property      `json:"profileProperties,omitempty"`
}

type ProfileResp struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Properties []Property `json:"properties,omitempty"`
}

type UserResp struct {
	ID       string     `json:"id"`
	Username string     `json:"username"`
	UsernameKnown bool  `json:"usernameKnown,omitempty"`
}

type Property struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SkinTextures struct {
	Timestamp  int64            `json:"timestamp"`
	ProfileID  string           `json:"profileId"`
	ProfileName string          `json:"profileName"`
	Textures   TexturesMap      `json:"textures"`
}

type TexturesMap struct {
	SKIN *SkinInfo `json:"SKIN,omitempty"`
	CAPE *SkinInfo `json:"CAPE,omitempty"`
}

type SkinInfo struct {
	URL      string            `json:"url"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type ErrorResp struct {
	Error         string `json:"error"`
	ErrorMessage  string `json:"errorMessage"`
	Cause         string `json:"cause,omitempty"`
}
