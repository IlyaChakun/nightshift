// Package training exists only to prove the module-level vuln gate.
package training

import jwt "github.com/dgrijalva/jwt-go"

// New returns an empty token; the vulnerable VerifyAudience is never called.
func New() *jwt.Token { return jwt.New(jwt.SigningMethodHS256) }
