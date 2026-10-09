package service

import (
	"fmt"
	"os"
	"strings"
)

type User struct {
	Name string
}

func ValidateEmail(email string) error {
	if strings.TrimSpace(email) == "" {
		return fmt.Errorf("empty email")
	}
	if !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email")
	}
	return nil
}

func ReadFirstByte(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err

	}
	defer f.Close()

	buf := make([]byte, 1)
	_, err = f.Read(buf)
	if err != nil {
		return "", err
	}

	return string(buf), nil
}

func UserLabel(u *User) (string, error) {
	return "User: " + u.Name, nil
}

func RemoveIfExists(path string) error {
	err := os.Remove(path)
	return err
}
