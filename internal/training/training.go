// Package training exists only in a throwaway PR that proves CI turns red.
package training

import "os"

// Remove deletes a file and ignores the error, which errcheck must flag.
func Remove(path string) {
	os.Remove(path)
}
