package auth

import (
	"errors"
	"regexp"
	"unicode"
)

var (
	// 经典的 RFC 5322 标准邮箱正则（简化版，覆盖 99% 场景）
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-z]{2,}$`)
)

// ValidateCredentials 负责所有注册时的格式校验
func ValidateCredentials(email, password string) error {
	// 1. 验证邮箱
	if email == "" {
		return errors.New("email is required")
	}
	if !emailRegex.MatchString(email) {
		return errors.New("invalid email format")
	}

	// 2. 验证密码长度
	if len(password) < 6 {
		return errors.New("password must be at least 6 characters long")
	}

	// 3. 验证密码复杂度
	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char): // 使用 IsDigit 替换 IsNumber 更准确
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return errors.New("password must contain at nil least one uppercase letter")
	}
	if !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}
	if !hasNumber {
		return errors.New("password must contain at least one number")
	}
	if !hasSpecial {
		return errors.New("password must contain at least one special character")
	}

	return nil
}
