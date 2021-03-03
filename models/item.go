package models

import (
	"regexp"
	"time"
)

var (
	itemURLRegexp  = regexp.MustCompile(`http[s]?://(?:[a-zA-Z]|[0-9]|[$-_@.&+]|[!*(),]|(?:%[0-9a-fA-F][0-9a-fA-F]))+`)
	itemNameRegexp = regexp.MustCompile(`^[\w\dА-Яа-я ():,.\-\\/&]{3,128}$`)
)

type Item struct {
	ID       int
	GroupID  int
	URL      string
	Name     string
	RepeatAt *time.Time
	Counter  int
}

func (i *Item) CheckURL(a string) bool {
	return len(a) <= 512 && itemURLRegexp.MatchString(a)
}

func (i *Item) CheckName(a string) bool {
	return itemNameRegexp.MatchString(a)
}
