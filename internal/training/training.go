// Package training exists only in a throwaway PR that proves CI turns red.
package training

import jwt "github.com/dgrijalva/jwt-go"

// HasAudience calls VerifyAudience, the symbol behind GO-2020-0017.
func HasAudience(claims jwt.MapClaims, aud string) bool {
	return claims.VerifyAudience(aud, true)
}
