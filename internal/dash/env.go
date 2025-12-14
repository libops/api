package dash

import "os"

// IsDevelopment returns true if the application is running in development mode
func IsDevelopment() bool {
	env := os.Getenv("LIBOPS_ENV")
	return env == "development"
}
