package main

import (
	"github.com/gungniir/telegram-quezlet-bot/database"
	"github.com/gungniir/telegram-quezlet-bot/telegram"
	log "github.com/sirupsen/logrus"
	"os"
)

func main() {
	token := os.Getenv("token")
	if token == "" {
		log.Fatalf("Failed to get token")
	}

	server := &telegram.TgServer{
		Config: &telegram.TgServerConfig{
			Token: token,
		},
	}

	db, err := database.NewPostgres("postgres://osat:OSA5vd8u@postgres/osat")

	if err != nil {
		log.WithError(err).Fatalf("Failed to connect to db")
	}

	log.Infof("Started listener")
	log.Fatalf("Telegram error: %s", server.ListenAndServe(db))
}
