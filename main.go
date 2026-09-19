package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

var tg *tgbotapi.BotAPI
var db *sql.DB

func buildAnswer(limit int) (string, error) {
	row, err := db.Query("SELECT user_agent, hwid, device_model FROM shadow_user LIMIT $1", limit)
	if err != nil {
		return "", err
	}
	defer row.Close()

	var lines []string

	for row.Next() {
		var userAgent string
		var hwid string
		var model string

		if err = row.Scan(&userAgent, &hwid, &model); err != nil {
			return "", err
		}
		line := fmt.Sprintf("%s | %s | %s", userAgent, hwid, model)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n\n\n"), nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	psql := os.Getenv("DB_LINK")
	adminID := os.Getenv("ADMIN_ID")
	int_adminID, err := strconv.ParseInt(adminID, 10, 64)
	if err != nil {
		log.Fatal(err)
	}
	adminID2 := os.Getenv("ADMIN_ID2")
	int_adminID2, err := strconv.ParseInt(adminID2, 10, 64)
	if err != nil {
		log.Fatal(err)
	}
	tgToken := os.Getenv("TOKEN")

	db, err = sql.Open("pgx", psql)
	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}

	tg, err = tgbotapi.NewBotAPI(tgToken)
	if err != nil {
		log.Panic(err)
	}
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	tg.Debug = true

	updates := tg.GetUpdatesChan(u)
	for update := range updates {
		if update.Message == nil {
			continue
		}
		var msg_text string
		switch update.Message.Command() {
		case "start":
			msg_text, err = buildAnswer(100)
			if err != nil {
				msg_text = "АШИБАЧКА"
			}
		default:
			msg_text = "ТАК НЕЛЬЗЯ"
		}
		if update.Message.Chat.ID != int_adminID2 && update.Message.Chat.ID != int_adminID {
			continue
		}
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, msg_text)
		_, err = tg.Send(msg)
		if err != nil {
			log.Println(err)
		}
	}
}
