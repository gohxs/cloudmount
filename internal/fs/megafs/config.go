package megafs

//Config  mega.yaml config file structure
type Config struct {
	// Fs service specific configuration here
	Credentials struct {
		Email    string
		Password string
	}
}
