cloudmount
=====================
Linux util to Mount cloud drives

**Table of Contents**

- [Installation](#installation)
- [Usage](#usage)
- [Example](#example)
- [Cloud services](#cloud-services)
  - [Google drive](#google-drive)
  - [Dropbox](#dropbox)
  - [Mega](#mega)
- [Signals](#signals)

<a name="installation"></a>
#### Installation
```bash
$ go get github.com/gohxs/cloudmount
```

<a name="usage"></a>
#### Usage
```bash
$ cloudmount -h

cloudmount-0.4-5-gf01e8fb - built: 2017-07-17 05:09:51 UTC

Usage: cloudmount [options] [<source>] <directory>

Source: can be json/yaml configuration file usually with credentials or cloud specific configuration

Options:
  -d	Run app in background
  -o string
    	uid=1000,gid=1000,ro=false
  -r duration
    	Timed cloud synchronization interval [if applied] (default 5s)
  -t string
    	which cloud service to use [gdrive] (default "gdrive")
  -v	Verbose log
  -vv
    	Extra Verbose log
  -w string
    	Work dir, path that holds configurations (default "$HOME/.cloudmount")

```
<a name="example"></a>
#### Example
```bash
# will default source file to $HOME/.cloudmount/gdrive.yaml
$ cloudmount -t gdrive /mnt/gdrive
# or 
$ cloudmount -t dropbox dropbox.yaml /mnt/dropbox
```

**Source config**
Configuration files/source can be written in following formats:   
* yaml
* json

<a name="cloud-services"></a>
#### Cloud services
* Google Drive
* Dropbox
* Mega

--------------

<a name="google-drive"></a>
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

sample _gdrive.yaml_ config:    
```yaml
client_secret:
  client_id: *Client ID*
  client_secret: *Client Secret*
```
```bash
$ cloudmount gdrive.yaml $HOME/mntpoint
```

Also it's possible to create the yaml file in home directory as 
__$HOME/.cloudmount/gdrive.yaml__
if &lt;source&gt; parameter is omitted it will default to this file

cloudmount gdrivefs will retrieve an oauth2 token and save in same file


<a name="dropbox"></a>
### Dropbox

Setup Dropbox client secrets:

https://www.dropbox.com/developers/apps

> 1. Click _Create App_ 
> 2. Select the API, type of access, and App name 
> 3. Use the values from _App key_ and _App secret_

sample _dropbox.yaml_ file:
```yaml
client_secret:
  client_id: *App Key*
  client_secret: *App secret*

```

```bash
$ cloudmount -t dropbox savedfile.yaml /mnt/point
```

On the first run a link will appear and it will request a token resulting from the link
<a name="mega"></a>
### Mega

For mega just create the yaml file with the following structure:
```yaml
type: mega
credentials: 
  email: *your mega account email*
  password: *your mega account password*
```

```bash
$ cloudmount -t mega config.yaml /mnt/point
```

--------------------

#### Signals
Signal | Action                                                                                               | ex
-------|------------------------------------------------------------------------------------------------------|-----------------
USR1   | Refreshes directory tree from file system                                                            | killall -USR1 cloudmount
HUP    | Perform a GC and shows memory usage <small>Works when its not running in daemon mode</small>         | killall -HUP cloudmount



#### Packages:
 * https://github.com/jacobsa/fuse -- fuse implementation (did some minor changes to support ARM)
 * https://github.com/dropbox/dropbox-sdk-go-unofficial -- dropbox  client (did some minor changes to fix an issue regarding non authorized urls)
 * https://github.com/t3rm1n4l/go-mega -- mega.co.nz
 * https://google.golang.org/api/drive/v3 -- google drive
 * https://github.com/gohxs/boiler -- code templating


#### TODO & IDEAS:
* Consider using github.com/codegangsta/cli
* Create test suit to implement new packages
* Caching: long term caching, maintain most used files locally until flush/change
* Add logging to syslog while on -d


