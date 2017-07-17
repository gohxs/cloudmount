#### TODO:   
* Fix google retry when error
* Improve logging system (is a bit messy)
* Create test cases
* Remove default gdrive and determine fs by arg[0] when possible
	* cloudmount.gdrive will mount gdrive
	* cloudmount.dropbox ..

#### Done:   
* Create and reference dropbox oauth doc
* Add verbosity levels (sometimes just want to log the driver and not fuse)
* Safemode flag not needed i supose 
* move client from fs's to service.go
* Sanitize error on basefs, file_container produces err, basefs produces fuse.E..


#### Ideas:
Sub mounting:

Current:  
cloudmount -t gdrive source.yaml destfolder

Idea:   
cloudmount -t gdrive gdrive.yaml/My\ Drive destfolder

Problem: 
Hard to figure  which part of the path is our configuration file so subsequent paths could be issue to find

Solution:
	Setup Root folder in configs

