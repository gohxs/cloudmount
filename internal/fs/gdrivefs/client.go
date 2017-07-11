// Oauth2 google api for Drive api

package gdrivefs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	drive "google.golang.org/api/drive/v3"

	"dev.hexasoftware.com/hxs/cloudmount/internal/core"

	"golang.org/x/oauth2"
)

type ServiceConfig struct {
	ClientSecret struct {
		ClientID     string   `json:"client_id"`
		ClientSecret string   `json:"client_secret"`
		RedirectURIs []string `json:"redirect_uris"`
		AuthURI      string   `json:"auth_uri"`
		TokenURI     string   `json:"token_uri"`
	} `json:"client_secret"`

	Auth *oauth2.Token `json:"auth"`
	/*struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		Expiry       string `json:"Expiry"`
	} `json:"auth"`*/
}

//func (d *GDriveFS) getClient(ctx context.Context, config *oauth2.Config) *http.Client {
//	cacheFile, err := d.tokenCacheFile()
//	if err != nil {
///		log.Fatalf("Unable to get path to cached credential file. %v", err)
//	}

/*if d.serviceConfig.Auth.AccessToken == "" {
		tok := getTokenFromWeb(config)
		d.serviceConfig.Auth = tok
		d.saveToken() // Save config actually
	}
	//tok, err := d.tokenFromFile(cacheFile)
	//if err != nil {
	//	tok = d.getTokenFromWeb(config)
	//	d.saveToken(cacheFile, tok)
	//	}
	return config.Client(ctx, tok)

//}

/*func (d *GDriveFS) tokenCacheFile() (string, error) {
	tokenCacheDir := d.config.HomeDir

	err := os.MkdirAll(tokenCacheDir, 0700)

	return filepath.Join(tokenCacheDir, url.QueryEscape("auth.json")), err

}*/

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

/*func (d *GDriveFS) tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	t := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(t)
	return t, err
}*/

//func (d *GDriveFS) saveToken(file string, token *oauth2.Token) {
/*func (d *GDriveFS) saveToken() { // Save credentials

	// Save token in SOURCE FILE
	log.Printf("Saving credential file to: %s\n", d.config.Source)
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v\n", err)
	}
	defer f.Close()

	json.NewEncoder(f).Encode(token)
}*/

func (d *GDriveFS) saveConfig() {

	data, err := json.MarshalIndent(d.serviceConfig, "  ", "  ")
	if err != nil {
		panic(err)
	}

	f, err := os.OpenFile(d.config.Source, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v\n", err)
	}
	defer f.Close()

	f.Write(data)
	f.Sync()

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
		RedirectURL:  d.serviceConfig.ClientSecret.RedirectURIs[0],
		Scopes:       []string{drive.DriveScope},
		Endpoint: oauth2.Endpoint{
			AuthURL:  d.serviceConfig.ClientSecret.AuthURI,
			TokenURL: d.serviceConfig.ClientSecret.TokenURI,
		},
	}
	// We can deal with oauthToken here too

	if d.serviceConfig.Auth == nil {
		tok := getTokenFromWeb(config)
		d.serviceConfig.Auth = tok
		d.saveConfig()
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
