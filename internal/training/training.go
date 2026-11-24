// Package training exists only in a throwaway PR that proves CI turns red.
package training

import jwt "github.com/dgrijalva/jwt-go"

// Parse verifies a token with a library that has a known advisory.
func Parse(token string, key []byte) (*jwt.Token, error) {
	return jwt.Parse(token, func(*jwt.Token) (any, error) { return key, nil })
}
