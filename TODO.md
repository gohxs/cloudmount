#### TODO:   
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

Original:  
cloudmount -t gdrive source.yaml destfolder

Idea:   
cloudmount -t gdrive gdrive.yaml/My\ Drive destfolder
