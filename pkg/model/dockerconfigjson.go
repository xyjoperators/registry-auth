package model

type DockerConfigJson struct {
	Auths map[string]DockerConfigAuth `json:"auths,omitempty"`
}

type DockerConfigAuth struct {
	Username string  `json:"username"`
	Password string  `json:"password"`
	Email    *string `json:"email,omitempty"`
	Auth     string  `json:"auth"`
}
