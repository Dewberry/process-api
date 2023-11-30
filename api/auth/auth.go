package auth

import (
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

type Claims struct {
	UserName    string              `json:"preferred_username"`
	Email       string              `json:"email"`
	RealmAccess map[string][]string `json:"realm_access"`
	Audience    []string            `json:"aud,omitempty"`
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
