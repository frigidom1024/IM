package util

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret = []byte("gochat-secret")

func Init(secret string) {
	jwtSecret = []byte(secret)
}

type Claims struct {
	UserID   int    `json:"user_id"`
	UserName string `json:"user_name"`
	jwt.RegisteredClaims
}

func GenerateToken(userID int, userName string, expireHours int) (string, error) {
	if len(jwtSecret) == 0 {

		return "", fmt.Errorf("jwt secret is empty")

	}

	claims := Claims{
		UserID:   userID,
		UserName: userName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * time.Duration(expireHours))),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func ParseToken(tokenString string) (*Claims, error) {
	if len(jwtSecret) == 0 {
		return nil, fmt.Errorf("jwt secret is empty")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}
