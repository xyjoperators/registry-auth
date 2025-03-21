package model

type DockerConfigJson struct {
	Auth map[string]DockerConfigAuth `json:"auth,omitempty"`
}

type DockerConfigAuth struct {
	Username string  `json:"username"`
	Password string  `json:"password"`
	Email    *string `json:"email,omitempty"`
	Auth     string  `json:"auth"`
}
