cloudmount
=====================

Linux util to Mount cloud drives

Usage:
```bash
$ go get dev.hexasoftware.com/hxs/cloudmount
$ cloudmount MOUNTPOINT

```

Support for:
* Google Drive







### Google Drive
07-05-2017


Setup Google client secrets:

[https://console.developers.google.com/apis/credentials] (https://console.developers.google.com/apis/credentials)

As of Google drive guidance:

>	Turn on the Drive API

>	1. Use [this wizard](https://console.developers.google.com/start/api?id=drive) to create or select a project in the Google Developers Console and automatically turn on the API. Click Continue, then Go to credentials.
>	2. On the Add credentials to your project page, click the Cancel button.
>	3. At the top of the page, select the OAuth consent screen tab. Select an Email address, enter a Product name if not already set, and click the Save button.
>	4. Select the Credentials tab, click the Create credentials button and select OAuth client ID.
>	5. Select the application type Other, enter the name "Drive API Quickstart", and click the Create button.
>	6. Click OK to dismiss the resulting dialog.
>	7. Click the file_download (Download JSON) button to the right of the client ID.

Copy the downloaded JSON file to home directory as:    
__$HOME/.gdrivemount/client_secret.json__   

#### Signals
Signal | Action                                                                                               | ex
-------|------------------------------------------------------------------------------------------------------|-----------------
USR1   | Refreshes directory tree from google drive                                                           | killall -USR1 gdrivemount
HUP    | Perform a GC and shows memory usage <small>Works when its not running in daemon mode</small>         | killall -HUP gdrivemount





####TODO:
* Assure concurrency support on inode/handle creation for gdrive
* Improve caching to refresh and save inodes
* Reverse package structure instead of gdrivemount/cmd/gdrivemount use this as main package and move logic to subpackage
* Use cloudmount -t gdrive -o uid, gid  MOUNTPOINT and add Support for other cloud services










TODO:
* create interface for drives
* manager to handle drivers
