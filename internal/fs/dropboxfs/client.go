package dropboxfs

import (
	"fmt"

	"dev.hexasoftware.com/hxs/cloudmount/internal/core"

	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	"golang.org/x/oauth2"
)

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token")

	fmt.Printf(
		`Go to the following link in your browser: 
----------------------------------------------------------------------------------------------
%v
----------------------------------------------------------------------------------------------

type the authorization code: `, authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(oauth2.NoContext, code)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}

	return tok
}

func (fs *DropboxFS) initClient() dropbox.Config {

	err := core.ParseConfig(fs.Config.Source, fs.serviceConfig)
	if err != nil {
		log.Fatalf("Unable to read <source>: %v", err)
	}

	config := &oauth2.Config{
		ClientID:     fs.serviceConfig.ClientSecret.ClientID,
		ClientSecret: fs.serviceConfig.ClientSecret.ClientSecret,
		RedirectURL:  "",
		Scopes:       []string{},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://www.dropbox.com/1/oauth2/authorize",
			TokenURL: "https://api.dropbox.com/1/oauth2/token",
		},
	}
	if fs.serviceConfig.Auth == nil {
		tok := getTokenFromWeb(config)
		fs.serviceConfig.Auth = tok
		core.SaveConfig(fs.Config.Source, fs.serviceConfig)
	}
	dbconfig := dropbox.Config{Token: fs.serviceConfig.Auth.AccessToken}

	return dbconfig
}
