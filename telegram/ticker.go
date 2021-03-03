package telegram

import (
	"context"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/gungniir/telegram-quezlet-bot/database"
	log "github.com/sirupsen/logrus"
	"time"
)

type Ticker struct {
	api *tgbotapi.BotAPI
	db  database.Database
}

func (t *Ticker) StartTicker(api *tgbotapi.BotAPI, db database.Database) {
	t.api = api
	t.db = db

	log.Info("Start ticker")

	go func() {
		for {
			now := time.Now().UTC()
			to := time.Date(now.Year(), now.Month(), now.Day()+1, 3, 0, 0, 0, time.UTC)

			log.Infof("Sleep until %s", to)

			time.Sleep(time.Until(to))

			t.tick()
		}
	}()
}

func (t *Ticker) tick() {
	log.Info("Tick")

	err := t.db.NextDayYesterdayItem(context.Background())

	if err != nil {
		log.WithError(err).Error("Failed to prolong yesterday packages")
	}

	items, err := t.db.GetTodayItems(context.Background())

	log.Infof("Today items count: %d", len(items))

	if err != nil {
		log.WithError(err).Error("Failed to get today items")
		return
	}

	itemIDs := make([]int, 0, 10)

	for _, item := range items {
		itemIDs = append(itemIDs, item.ID)
	}

	chatIDs, err := t.db.GetChatIDsByItemIDs(context.Background(), itemIDs)

	log.Infof("Today chats count: %d", len(chatIDs))

	if err != nil {
		log.WithError(err).Error("Failed to get chatIDs")
		return
	}

	{
		notified := make(map[int64]bool)

		for _, chatIDs := range chatIDs {
			for _, chatID := range chatIDs {
				if notified[chatID] {
					continue
				}
				notified[chatID] = true

				m := tgbotapi.NewMessage(chatID, "Доброе утро! Соскучились по модулям? А они-то как по вас?)\nВ общем, пора учиться :)")

				_, err := t.api.Send(m)

				if err != nil {
					log.WithError(err).Warn("Failed to send message to chat")
				}
			}
		}
	}

	for _, item := range items {
		for _, chatID := range chatIDs[item.ID] {
			m := tgbotapi.NewMessage(chatID,
				fmt.Sprintf("%s\n[Тыц по ссылке](%s)", item.Name, item.URL),
			)

			m.DisableWebPagePreview = true
			m.DisableNotification = true
			m.ParseMode = tgbotapi.ModeMarkdown
			m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("Повторили!", fmt.Sprintf("SETOK:%d.%d", item.ID, item.Counter)),
				),
			)

			_, err := t.api.Send(m)

			if err != nil {
				log.WithError(err).Warn("Failed to send message to chat")
			}
		}
	}
}
