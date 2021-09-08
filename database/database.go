package database

import (
	"context"
	"github.com/gungniir/telegram-quezlet-bot/models"
)

type Database interface {
	CreateGroup(ctx context.Context, passwordHash string) (*models.Group, error)
	GetGroup(ctx context.Context, groupID int) (*models.Group, error)

	AddUserToGroup(ctx context.Context, userID, groupID int) error
	RemoveUserFromGroup(ctx context.Context, userID, groupID int) error
	GetUserGroups(ctx context.Context, userID int) ([]*models.Group, error)

	GetItemsByGroupID(ctx context.Context, groupID int) ([]*models.Item, error)
	GetTodayItems(ctx context.Context) ([]*models.Item, error)
	CreateItem(ctx context.Context, groupID int, url, name string) (*models.Item, error)
	ProlongByItemIDWithCheck(ctx context.Context, itemID, counter int) error
	ProlongYesterdayItem(ctx context.Context) error

	SetChatIDByUserID(ctx context.Context, chatID int64, userID int) error
	GetChatIDsByUserIDs(ctx context.Context, userIDs []int) (map[int]int64, error)
	GetChatIDsByItemIDs(ctx context.Context, userIDs []int) (map[int][]int64, error)
}
