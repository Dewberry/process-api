package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt"
	"github.com/labstack/echo/v4"
)

type AuthStrategy interface {
	ValidateToken(tokenString string) (*Claims, error)
	ValidateUser(c echo.Context, claims *Claims) error
	SetUserRolesHeader(c echo.Context, claims *Claims) error
}

type Audience []string

// aud in token can be []string or string, therefore we need a custom unmarshaler.
// jwt package uses the json package for unmarshalling JSON into Go structs.
func (a *Audience) UnmarshalJSON(data []byte) error {
	// Try to unmarshal data into a slice of strings
	var audienceSlice []string
	if err := json.Unmarshal(data, &audienceSlice); err == nil {
		*a = audienceSlice
		return nil
	}

	// If the above fails, try to unmarshal as a single string
	var singleAud string
	if err := json.Unmarshal(data, &singleAud); err != nil {
		return err
	}

	*a = []string{singleAud}
	return nil
}

type Claims struct {
	UserName    string              `json:"preferred_username"`
	Email       string              `json:"email"`
	RealmAccess map[string][]string `json:"realm_access"`
	Audience    Audience            `json:"aud,omitempty"`
	jwt.StandardClaims
}

func overlap(s1 []string, s2 []string) bool {
	for _, x := range s1 {
		for _, y := range s2 {
			if x == y {
				return true
			}
		}
	}
	return false
}

// Middleware
func Authorize(strategy AuthStrategy) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHead := c.Request().Header.Get("Authorization")
			// Check if the Authorization header is missing or not in the expected format
			if authHead == "" || !strings.HasPrefix(authHead, "Bearer ") {
				return c.JSON(http.StatusUnauthorized, "missing or invalid authorization header")
			}

			tokenString := strings.Split(authHead, "Bearer ")[1]
			if tokenString == "" {
				return c.JSON(http.StatusUnauthorized, "missing authorization header")
			}

			claims, err := strategy.ValidateToken(tokenString)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, err.Error())
			}

			err = strategy.ValidateUser(c, claims)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, err.Error())
			}

			err = strategy.SetUserRolesHeader(c, claims)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, err.Error())
			}

			return next(c)
		}
	}
}
