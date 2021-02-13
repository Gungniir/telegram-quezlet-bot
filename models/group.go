package models

import (
	"crypto/sha256"
	"fmt"
	"regexp"
)

var (
	groupPasswordRegexp = regexp.MustCompile(`^[\w\d]{3,16}$`)
)

const salt = "ALALALA"

type Group struct {
	ID           int
	PasswordHash string
}

func (g *Group) CheckPassword(a string) bool {
	return groupPasswordRegexp.MatchString(a)
}

func (g *Group) HashPassword(a string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(a+salt)))
}
