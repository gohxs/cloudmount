cloudmount
=====================

Linux util to Mount cloud drives

####Usage:
```bash
$ cloudmount -h

cloudmount-0.3-3-gadf4880 - built: 2017-07-11 01:34:58 UTC

Usage: cloudmount [options] [<source>] <directory>

Source: can be json/yaml configuration file usually with credentials or cloud specific configuration

Options:
  -d	Run app in background
  -o string
    	uid,gid ex: -o uid=1000,gid=0 
  -r duration
    	Timed cloud synchronization interval [if applied] (default 2m0s)
  -t string
    	which cloud service to use [gdrive] (default "gdrive")
  -v	Verbose log
  -w string
    	Work dir, path that holds configurations (default "<homedir>/.cloudmount")
```


#### Example:
```bash
$ go get dev.hexasoftware.com/hxs/cloudmount
# will default source file to $HOME/.cloudmount/gdrive.json
$ cloudmount MOUNTPOINT
# or 
$ cloudmount gdrive.json MOUNTPOINT

```
#### Source config:
Configuration files/source can be written in following formats:
* json
* yaml

#### Support for:
* Google Drive


### Google Drive

Setup Google client secrets:

https://console.developers.google.com/apis/credentials

>	Turn on the Drive API

>	1. Use [this wizard](https://console.developers.google.com/start/api?id=drive) to create or select a project in the Google Developers Console and automatically turn on the API. Click Continue, then Go to credentials.
>	2. On the Add credentials to your project page, click the Cancel button.
>	3. At the top of the page, select the OAuth consent screen tab. Select an Email address, enter a Product name if not already set, and click the Save button.
>	4. Select the Credentials tab, click the Create credentials button and select OAuth client ID.
>	5. Select the application type Other, enter the name "Drive API Quickstart", and click the Create button.
>	6. With the result dialog, copy clientID and client secret and create json file as shown in example (this can be retrieved any time by clicking on the api key)

sample *gdrive.json* config:    
```json
{
  "client_secret": {
   "client_id": "CLIENTID",
   "client_secret": "CLIENTSECRET"
  }
}
```
or yaml format:
```yaml
client_secret:
  client_id: CLIENTID
  client_secret: CLIENTSECRET
```

```bash
$ cloudmount gdrive.json $HOME/mntpoint
```

Also it's possible to create the json/yaml file in home directory as 
__$HOME/.cloudmount/gdrive.json__
if &lt;source&gt; parameter is ommited it will default to this file


cloudmount gdrivefs will retrieve an oauth2 token and save in same file



#### Signals
Signal | Action                                                                                               | ex
-------|------------------------------------------------------------------------------------------------------|-----------------
USR1   | Refreshes directory tree from file system                                                            | killall -USR1 gdrivemount
HUP    | Perform a GC and shows memory usage <small>Works when its not running in daemon mode</small>         | killall -HUP gdrivemount



#### TODO & IDEAS:
* Consider using github.com/codegangsta/cli
* Create test suit to implement new packages
* GDrive: long term caching, maintain most used files locally until flush/change
* File_container can be common for all FS?
* Define what should be common per FS and create an interface for implementations


#### Plan:

Create a common structure for driver
// Driver needs populate list somehow
