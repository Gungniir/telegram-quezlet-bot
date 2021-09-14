package main

import (
	"github.com/gungniir/telegram-quezlet-bot/database"
	"github.com/gungniir/telegram-quezlet-bot/telegram"
	log "github.com/sirupsen/logrus"
	"os"
	"time"
)

func main() {
	token := os.Getenv("token")
	if token == "" {
		log.Fatalf("Failed to get token")
	}

	server := &telegram.TgServer{
		Config: &telegram.TgServerConfig{
			Token:    token,
			Timezone: time.FixedZone("Asia/Krasnoyarsk", 60*60*7),
		},
	}

	db, err := database.NewPostgres("postgres://osat:OSA5vd8u@postgres/osat", 7)

	if err != nil {
		log.WithError(err).Fatalf("Failed to connect to db")
	}

	log.Infof("Started listener")
	log.Fatalf("Telegram error: %s", server.ListenAndServe(db))
}
