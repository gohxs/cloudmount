package dropboxfs

import "golang.org/x/oauth2"

//Silimar to gdrive
type Config struct {
	ClientSecret struct {
		ClientID     string `json:"client_id" yaml:"client_id"`
		ClientSecret string `json:"client_secret" yaml:"client_secret"`
	} `json:"client_secret" yaml:"client_secret"`

	Auth *oauth2.Token `json:"auth" yaml:"auth"`
}
