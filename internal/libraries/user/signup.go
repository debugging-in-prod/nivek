package user

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

type SignupRequest struct {
	Username string `db:"username" json:"username"`
	Email    string `db:"email" json:"email"`
	Password string `db:"password" json:"password"`
}

func (s *nivekUserServiceImpl) Signup(request SignupRequest) (bool, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		return false, fmt.Errorf("error hashing password: %v", err)
	}

	request.Password = string(hashedPassword)

	result, err := s.userTable.Insert(request)

	if err != nil {
		return false, fmt.Errorf("error inserting user: %s", err.Error())
	}

	logrus.Infof("User %d created", result.ID())
	return true, nil
}
