package oauth2util

import (
	"fmt"
	"log"

	"golang.org/x/oauth2"
)

//GetTokenFromWeb shows link to user, and requests the informed token
func GetTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	//authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
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
