// Package auth 实现 JWT 鉴权与密码管理
// JWT Secret 每次启动通过 crypto/rand 动态生成，绝不硬编码
package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var jwtSecret []byte

// InitSecret 每次程序启动时动态生成 32 字节强随机密钥
func InitSecret() error {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("生成 JWT Secret 失败: %w", err)
	}
	jwtSecret = secret
	return nil
}

// HashPassword 使用 Bcrypt 对明文密码加盐哈希
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("密码哈希失败: %w", err)
	}
	return string(bytes), nil
}

// CheckPassword 验证明文密码是否与 Bcrypt 哈希匹配
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// Claims 自定义 JWT Claims，包含用户名和 Token 版本
type Claims struct {
	Username     string `json:"username"`
	TokenVersion int    `json:"token_version"`
	jwt.RegisteredClaims
}

// GenerateToken 签发 JWT（有效期 24 小时）
func GenerateToken(username string, tokenVersion int) (string, error) {
	claims := Claims{
		Username:     username,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ValidateToken 校验 JWT 并返回 Claims
func ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{},
		func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("意外的签名方法: %v", token.Header["alg"])
			}
			return jwtSecret, nil
		})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("无效的 Token")
	}
	return claims, nil
}

// GetSecretBytes 暴露 jwtSecret 给初始化验证（安全红线：仅用于检测是否已初始化）
func GetSecretBytes() []byte {
	return jwtSecret
}
