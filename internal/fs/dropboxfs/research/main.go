package main

import (
	"log"

	"github.com/stacktic/dropbox"
)

func main() {
	var err error
	var db *dropbox.Dropbox

	clientid := "ycsj9tcm7bqdutw"
	clientsecret := "rtlg6f5jfgau80s"
	//token := "Iw62s3Vg5sAAAAAAAAAuPeAaIstFQmi3iJ659RMTlaL_xcV7FPnYMMwtNdMNEII5"
	token := "Iw62s3Vg5sAAAAAAAAAuP-725bDtvizWhH-OhxyvxjSgeNXYxIrL44siRqpw4ZNJ"

	// 1. Create a new dropbox object.
	db = dropbox.NewDropbox()

	// 2. Provide your clientid and clientsecret (see prerequisite).
	db.SetAppInfo(clientid, clientsecret)

	// 3. Provide the user token.
	// This method will ask the user to visit an URL and paste the generated code.
	/*if err = db.Auth(); err != nil {
		fmt.Println(err)
		return
	}
	token := db.AccessToken() // You can now retrieve the token if you want.*/
	log.Println("Tok:", token)
	db.SetAccessToken(token)

	// 4. Send your commands.
	// In this example, you will create a new folder named "demo".
	log.Println("Commands")
	acct, err := db.GetAccountInfo()
	errCheck(err)
	log.Println(acct.Country)
	log.Println(acct.DisplayName)
	log.Println(acct.QuotaInfo.Normal)

	entry, err := db.Metadata("/", true, false, "", "", 0)
	errCheck(err)
	log.Println("Entries:", len(entry.Contents))

	for _, e := range entry.Contents {
		log.Println("Entry:", e.Hash, e.Path, e.Bytes)
	}

	dm := db.NewDatastoreManager()
	ds, err := dm.ListDatastores()
	errCheck(err)
	log.Println("Data stores:", ds)

	//	folder := "demo"
	/*if _, err = db.CreateFolder(folder); err != nil {
		fmt.Printf("Error creating folder %s: %s\n", folder, err)
	} else {
		fmt.Printf("Folder %s successfully created\n", folder)
	}*/
}

func errCheck(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
