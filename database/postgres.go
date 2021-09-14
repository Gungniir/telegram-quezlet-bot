package database

import (
	"context"
	"fmt"
	"github.com/gungniir/telegram-quezlet-bot/models"
	"github.com/jackc/pgx/v4/pgxpool"
	"time"
)

type Postgres struct {
	pool *pgxpool.Pool
	loc  int
}

func NewPostgres(connString string, loc int) (*Postgres, error) {
	conn, err := pgxpool.Connect(context.Background(), connString)

	if err != nil {
		return nil, err
	}

	_, err = conn.Exec(context.Background(), fmt.Sprintf(`SET TIME ZONE %d`, loc))

	if err != nil {
		return nil, err
	}

	p := &Postgres{
		pool: conn,
		loc:  loc,
	}

	return p, nil
}

func (p *Postgres) GetDate(ctx context.Context) (*time.Time, error) {
	pool := p.pool

	rows, err := pool.Query(ctx, `SELECT current_date`)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	rows.Next()

	current := new(time.Time)

	err = rows.Scan(current)

	return current, err
}

func (p *Postgres) CreateGroup(ctx context.Context, passwordHash string) (*models.Group, error) {
	pool := p.pool
	rows, err := pool.Query(ctx, `INSERT INTO groups(password_hash) VALUES ($1) RETURNING id`, passwordHash)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	rows.Next()

	group := &models.Group{
		PasswordHash: passwordHash,
	}
	err = rows.Scan(&group.ID)

	if err != nil {
		return nil, err
	}

	return group, nil
}

func (p *Postgres) AddUserToGroup(ctx context.Context, userID, groupID int) error {
	pool := p.pool

	_, err := pool.Exec(ctx, `INSERT INTO groups_users_links(user_id, group_id) VALUES ($1, $2)`, userID, groupID)

	return err
}

func (p *Postgres) GetUserGroups(ctx context.Context, userID int) ([]*models.Group, error) {
	pool := p.pool
	rows, err := pool.Query(ctx, `SELECT g.* FROM groups_users_links INNER JOIN groups g on g.id = groups_users_links.group_id WHERE user_id = $1`, userID)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var groups []*models.Group

	for rows.Next() {
		group := &models.Group{}

		err = rows.Scan(&group.ID, &group.PasswordHash)

		if err != nil {
			return nil, err
		}

		groups = append(groups, group)
	}

	return groups, nil
}

func (p *Postgres) RemoveUserFromGroup(ctx context.Context, userID, groupID int) error {
	pool := p.pool

	_, err := pool.Exec(ctx, `DELETE FROM groups_users_links WHERE user_id = $1 AND group_id = $2`, userID, groupID)

	return err
}

func (p *Postgres) GetGroup(ctx context.Context, groupID int) (*models.Group, error) {
	pool := p.pool

	rows, err := pool.Query(ctx, `SELECT * FROM groups WHERE id = $1`, groupID)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}

	group := &models.Group{}

	err = rows.Scan(&group.ID, &group.PasswordHash)

	if err != nil {
		return nil, err
	}

	return group, nil
}

func (p *Postgres) GetItemsByGroupID(ctx context.Context, groupID int) ([]*models.Item, error) {
	pool := p.pool

	rows, err := pool.Query(ctx, `SELECT * FROM items WHERE group_id = $1 ORDER BY repeat_at`, groupID)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	items := make([]*models.Item, 0)

	for rows.Next() {
		item := &models.Item{}

		err = rows.Scan(&item.ID, &item.URL, &item.Name, &item.GroupID, &item.RepeatAt, &item.Counter)

		if err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, nil
}

func (p *Postgres) CreateItem(ctx context.Context, groupID int, url, name string) (*models.Item, error) {
	pool := p.pool

	rows, err := pool.Query(ctx, `INSERT INTO items(url, name, group_id) VALUES ($2, $3, $1) RETURNING *`, groupID, url, name)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	rows.Next()

	item := &models.Item{}

	err = rows.Scan(&item.ID, &item.URL, &item.Name, &item.GroupID, &item.RepeatAt, &item.Counter)

	if err != nil {
		return nil, err
	}

	return item, nil
}

func (p *Postgres) SetChatIDByUserID(ctx context.Context, chatID int64, userID int) error {
	pool := p.pool

	_, err := pool.Exec(ctx, `INSERT INTO user_chat_links(user_id, chat_id) VALUES($1, $2)`, userID, chatID)

	return err
}

func (p *Postgres) GetChatIDsByUserIDs(ctx context.Context, userIDs []int) (map[int]int64, error) {
	pool := p.pool

	ids := make(map[int]int64, len(userIDs))

	rows, err := pool.Query(ctx, `SELECT chat_id, user_id from user_chat_links WHERE user_id = ANY($1)`, userIDs)

	if err != nil {
		return ids, err
	}

	defer rows.Close()

	for rows.Next() {
		var (
			chatID int64
			userID int
		)

		err = rows.Scan(&chatID, &userID)

		if err != nil {
			return nil, err
		}

		ids[userID] = chatID
	}

	return ids, err
}

func (p *Postgres) GetTodayItems(ctx context.Context) ([]*models.Item, error) {
	pool := p.pool

	rows, err := pool.Query(ctx, `SELECT * FROM items WHERE repeat_at = current_date ORDER BY group_id, id`)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	items := make([]*models.Item, 0)

	for rows.Next() {
		item := &models.Item{}

		err = rows.Scan(&item.ID, &item.URL, &item.Name, &item.GroupID, &item.RepeatAt, &item.Counter)

		if err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, nil
}

func (p *Postgres) GetChatIDsByItemIDs(ctx context.Context, itemIDs []int) (map[int][]int64, error) {
	pool := p.pool

	rows, err := pool.Query(ctx, `SELECT id, unnest(chat_ids) FROM item_chats WHERE id = ANY($1)`, itemIDs)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	items := make(map[int][]int64)

	for rows.Next() {
		var (
			id     int
			chatID int64
		)

		err = rows.Scan(&id, &chatID)

		if err != nil {
			return nil, err
		}

		items[id] = append(items[id], chatID)
	}

	return items, nil
}

func (p *Postgres) ProlongByItemIDWithCheck(ctx context.Context, itemID, counter int) error {
	pool := p.pool

	_, err := pool.Exec(ctx, `UPDATE items SET repeat_at = current_date + (SELECT add FROM prolong WHERE count = (SELECT counter FROM items WHERE id = $1 LIMIT 1) LIMIT 1), counter = $2 + 1 WHERE id = $1 AND counter = $2`, itemID, counter)

	return err
}

func (p *Postgres) ProlongYesterdayItem(ctx context.Context) error {
	pool := p.pool

	_, err := pool.Exec(ctx, `UPDATE items SET repeat_at = current_date WHERE repeat_at < current_date`)

	return err
}
