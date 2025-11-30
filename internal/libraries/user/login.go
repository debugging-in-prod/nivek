package user

import (
	"fmt"

	"github.com/upper/db/v4"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *nivekUserServiceImpl) Login(request LoginRequest) (*User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("error during login attempt - %s", err)
	}

	var usr User
	err = s.userTable.Find(db.Cond{
		"email":    request.Email,
		"password": string(hashedPassword),
	}).One(&usr)

	if err != nil {
		return nil, fmt.Errorf(
			"error during login attempt - %s: %s",
			request.Email,
			err.Error(),
		)
	}

	return &usr, nil
}
