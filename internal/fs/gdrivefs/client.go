// Oauth2 google api for Drive api

package gdrivefs

import (
	"context"
	"fmt"

	drive "google.golang.org/api/drive/v3"

	"dev.hexasoftware.com/hxs/cloudmount/internal/core"

	"golang.org/x/oauth2"
)

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

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

// Init driveService
func (d *GDriveFS) initClient() {

	//configPath := d.config.HomeDir

	ctx := context.Background() // Context from GDriveFS

	log.Println("Initializing gdrive service")
	log.Println("Source config:", d.config.Source)

	err := core.ParseConfig(d.config.Source, d.serviceConfig)

	//b, err := ioutil.ReadFile(d.config.Source)

	//b, err := ioutil.ReadFile(filepath.Join(configPath, "client_secret.json"))
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}
	config := &oauth2.Config{
		ClientID:     d.serviceConfig.ClientSecret.ClientID,
		ClientSecret: d.serviceConfig.ClientSecret.ClientSecret,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob", //d.serviceConfig.ClientSecret.RedirectURIs[0],
		Scopes:       []string{drive.DriveScope},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",  //d.serviceConfig.ClientSecret.AuthURI,
			TokenURL: "https://accounts.google.com/o/oauth2/token", //d.serviceConfig.ClientSecret.TokenURI,
		},
	}
	// We can deal with oauthToken here too

	if d.serviceConfig.Auth == nil {
		tok := getTokenFromWeb(config)
		d.serviceConfig.Auth = tok
		core.SaveConfig(d.config.Source, d.serviceConfig)
	}

	/*config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file: %v", err)
	}*/

	client := config.Client(ctx, d.serviceConfig.Auth)
	d.client, err = drive.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve drive Client: %v", err)
	}

}
