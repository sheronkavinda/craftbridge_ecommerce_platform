package auth

import "time"

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Name         string
	Status       string
	Roles        []string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type RegisterInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ForgotPasswordInput struct {
	Email string `json:"email"`
}

type ResetPasswordInput struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}
