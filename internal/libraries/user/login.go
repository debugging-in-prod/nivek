package user

import (
	"fmt"
	"log"

	"github.com/upper/db/v4"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *nivekUserServiceImpl) Login(request LoginRequest) (*User, error) {
	var usr User

	// Find the user by email only
	err := s.userTable.Find(db.Cond{"email": request.Email}).One(&usr)
	if err != nil {
		return nil, fmt.Errorf("user not found: %s", request.Email)
	}

	err = bcrypt.CompareHashAndPassword([]byte(usr.Password), []byte(request.Password))
	if err != nil {
		return nil, fmt.Errorf("invalid password for user %s", request.Email)
	}

	return &usr, nil
}
