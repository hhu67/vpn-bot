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

// Глобальные переменные для Telegram бота и подключения к базе данных
var tg *tgbotapi.BotAPI
var db *sql.DB

// Константы для пагинации
const usersPerPage = 10

// Структура для хранения данных пользователя
type User struct {
	UserAgent   string
	HWID        string
	DeviceModel string
	RealName    string
	Block       bool
}

// getUsersCount возвращает общее количество пользователей
func getUsersCount() (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM shadow_user").Scan(&count)
	return count, err
}

// getUsers получает список пользователей с пагинацией
func getUsers(offset, limit int) ([]User, error) {
	rows, err := db.Query("SELECT user_agent, hwid, device_model, real_name, block FROM shadow_user ORDER BY hwid LIMIT $1 OFFSET $2", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		if err = rows.Scan(&user.UserAgent, &user.HWID, &user.DeviceModel, &user.RealName, &user.Block); err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

// getUserByHWID получает информацию о пользователе по HWID
func getUserByHWID(hwid string) (*User, error) {
	var user User
	err := db.QueryRow("SELECT user_agent, hwid, device_model, real_name, block FROM shadow_user WHERE hwid=$1", hwid).
		Scan(&user.UserAgent, &user.HWID, &user.DeviceModel, &user.RealName, &user.Block)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// blockUser блокирует пользователя по HWID
func blockUser(hwid string) error {
	result, err := db.Exec("UPDATE shadow_user SET block = true WHERE hwid=$1", hwid)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// unblockUser разблокирует пользователя по HWID
func unblockUser(hwid string) error {
	result, err := db.Exec("UPDATE shadow_user SET block = false WHERE hwid=$1", hwid)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// createMainMenu создает главное меню с inline кнопками
func createMainMenu() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Список пользователей", "list:0"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📊 Статистика", "stats"),
		),
	)
}

// createUsersListMenu создает меню со списком пользователей
func createUsersListMenu(page int) (string, tgbotapi.InlineKeyboardMarkup, error) {
	offset := page * usersPerPage
	users, err := getUsers(offset, usersPerPage)
	if err != nil {
		return "", tgbotapi.InlineKeyboardMarkup{}, err
	}

	totalCount, err := getUsersCount()
	if err != nil {
		return "", tgbotapi.InlineKeyboardMarkup{}, err
	}

	// Формируем текст сообщения
	text := fmt.Sprintf("📋 *Список пользователей*\n\nСтраница %d\nВсего пользователей: %d\n\nВыберите пользователя:", page+1, totalCount)

	// Формируем inline кнопки с пользователями
	var rows [][]tgbotapi.InlineKeyboardButton
	for i, user := range users {
		num := offset + i + 1
		// Показываем real_name в списке
		displayName := user.RealName
		if displayName == "" {
			displayName = "Без имени"
		}
		buttonText := fmt.Sprintf("%d. %s", num, displayName)
		if user.Block {
			buttonText += " 🔒"
		}
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(buttonText, fmt.Sprintf("user:%s", user.HWID)),
		)
		rows = append(rows, row)
	}

	// Кнопки навигации
	var navButtons []tgbotapi.InlineKeyboardButton
	if page > 0 {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", fmt.Sprintf("list:%d", page-1)))
	}
	if offset+usersPerPage < totalCount {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("Вперёд ➡️", fmt.Sprintf("list:%d", page+1)))
	}
	if len(navButtons) > 0 {
		rows = append(rows, navButtons)
	}

	// Кнопка возврата в главное меню
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🏠 Главное меню", "main"),
	))

	return text, tgbotapi.NewInlineKeyboardMarkup(rows...), nil
}

// createUserDetailMenu создает меню с информацией о пользователе
func createUserDetailMenu(hwid string) (string, tgbotapi.InlineKeyboardMarkup, error) {
	user, err := getUserByHWID(hwid)
	if err != nil {
		return "", tgbotapi.InlineKeyboardMarkup{}, err
	}

	// Формируем текст с информацией о пользователе
	status := "✅ Активен"
	if user.Block {
		status = "🔒 Заблокирован"
	}

	// Показываем real_name как основное имя
	displayName := user.RealName
	if displayName == "" {
		displayName = "Не указано"
	}

	text := fmt.Sprintf(
		"👤 *Информация о пользователе*\n\n"+
			"*Имя:* `%s`\n"+
			"*User Agent:* `%s`\n"+
			"*HWID:* `%s`\n"+
			"*Модель устройства:* `%s`\n"+
			"*Статус:* %s",
		displayName, user.UserAgent, user.HWID, user.DeviceModel, status,
	)

	// Формируем кнопки
	var actionButton tgbotapi.InlineKeyboardButton
	if user.Block {
		actionButton = tgbotapi.NewInlineKeyboardButtonData("✅ Разблокировать", fmt.Sprintf("unblock:%s", hwid))
	} else {
		actionButton = tgbotapi.NewInlineKeyboardButtonData("🔒 Заблокировать", fmt.Sprintf("block:%s", hwid))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(actionButton),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ К списку", "list:0"),
			tgbotapi.NewInlineKeyboardButtonData("🏠 Главное меню", "main"),
		),
	)

	return text, keyboard, nil
}

// createStatsMenu создает меню со статистикой
func createStatsMenu() (string, tgbotapi.InlineKeyboardMarkup, error) {
	totalCount, err := getUsersCount()
	if err != nil {
		return "", tgbotapi.InlineKeyboardMarkup{}, err
	}

	var blockedCount int
	err = db.QueryRow("SELECT COUNT(*) FROM shadow_user WHERE block = true").Scan(&blockedCount)
	if err != nil {
		return "", tgbotapi.InlineKeyboardMarkup{}, err
	}

	activeCount := totalCount - blockedCount

	text := fmt.Sprintf(
		"📊 *Статистика*\n\n"+
			"Всего пользователей: *%d*\n"+
			"Активных: *%d* ✅\n"+
			"Заблокированных: *%d* 🔒",
		totalCount, activeCount, blockedCount,
	)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏠 Главное меню", "main"),
		),
	)

	return text, keyboard, nil
}

func main() {
	// Загружаем переменные окружения из .env файла
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Получаем данные подключения из переменных окружения
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

	// Подключаемся к базе данных PostgreSQL
	db, err = sql.Open("pgx", psql)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Проверяем соединение с базой данных
	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}

	// Инициализируем Telegram бота
	tg, err = tgbotapi.NewBotAPI(tgToken)
	if err != nil {
		log.Panic(err)
	}
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	tg.Debug = true

	log.Println("Бот запущен...")

	// Получаем канал обновлений от Telegram
	updates := tg.GetUpdatesChan(u)
	for update := range updates {
		// Обработка текстовых команд
		if update.Message != nil {
			// КРИТИЧЕСКИ ВАЖНО: Проверяем, что команду отправил админ
			if update.Message.Chat.ID != int_adminID && update.Message.Chat.ID != int_adminID2 {
				continue
			}

			// Обрабатываем только команду /start
			if update.Message.Command() == "start" {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "🏠 *Главное меню*\n\nВыберите действие:")
				msg.ParseMode = "Markdown"
				msg.ReplyMarkup = createMainMenu()
				_, err = tg.Send(msg)
				if err != nil {
					log.Printf("Ошибка отправки сообщения: %v", err)
				}
			}
			continue
		}

		// Обработка callback запросов (нажатия на inline кнопки)
		if update.CallbackQuery != nil {
			callback := update.CallbackQuery

			// КРИТИЧЕСКИ ВАЖНО: Проверяем, что callback от админа
			if callback.Message.Chat.ID != int_adminID && callback.Message.Chat.ID != int_adminID2 {
				// Отправляем уведомление о том, что доступ запрещен
				callbackConfig := tgbotapi.NewCallback(callback.ID, "❌ Доступ запрещен")
				tg.Send(callbackConfig)
				continue
			}

			// Парсим callback data
			data := callback.Data
			parts := strings.Split(data, ":")

			var text string
			var keyboard tgbotapi.InlineKeyboardMarkup
			var errMsg string

			switch parts[0] {
			case "main":
				// Главное меню
				text = "🏠 *Главное меню*\n\nВыберите действие:"
				keyboard = createMainMenu()

			case "list":
				// Список пользователей
				page := 0
				if len(parts) > 1 {
					page, _ = strconv.Atoi(parts[1])
				}
				text, keyboard, err = createUsersListMenu(page)
				if err != nil {
					log.Printf("Ошибка получения списка пользователей: %v", err)
					errMsg = "❌ Ошибка получения списка"
				}

			case "user":
				// Информация о пользователе
				if len(parts) > 1 {
					hwid := parts[1]
					text, keyboard, err = createUserDetailMenu(hwid)
					if err != nil {
						log.Printf("Ошибка получения информации о пользователе: %v", err)
						errMsg = "❌ Пользователь не найден"
					}
				}

			case "block":
				// Блокировка пользователя
				if len(parts) > 1 {
					hwid := parts[1]
					err = blockUser(hwid)
					if err != nil {
						log.Printf("Ошибка блокировки пользователя: %v", err)
						errMsg = "❌ Ошибка блокировки"
					} else {
						// Обновляем информацию о пользователе
						text, keyboard, err = createUserDetailMenu(hwid)
						if err != nil {
							errMsg = "✅ Заблокировано, но ошибка обновления"
						}
					}
				}

			case "unblock":
				// Разблокировка пользователя
				if len(parts) > 1 {
					hwid := parts[1]
					err = unblockUser(hwid)
					if err != nil {
						log.Printf("Ошибка разблокировки пользователя: %v", err)
						errMsg = "❌ Ошибка разблокировки"
					} else {
						// Обновляем информацию о пользователе
						text, keyboard, err = createUserDetailMenu(hwid)
						if err != nil {
							errMsg = "✅ Разблокировано, но ошибка обновления"
						}
					}
				}

			case "stats":
				// Статистика
				text, keyboard, err = createStatsMenu()
				if err != nil {
					log.Printf("Ошибка получения статистики: %v", err)
					errMsg = "❌ Ошибка получения статистики"
				}

			default:
				errMsg = "❌ Неизвестная команда"
			}

			// Отправляем callback ответ
			if errMsg != "" {
				callbackConfig := tgbotapi.NewCallback(callback.ID, errMsg)
				tg.Send(callbackConfig)
			} else {
				callbackConfig := tgbotapi.NewCallback(callback.ID, "")
				tg.Send(callbackConfig)

				// Редактируем сообщение с новым текстом и клавиатурой
				editMsg := tgbotapi.NewEditMessageText(
					callback.Message.Chat.ID,
					callback.Message.MessageID,
					text,
				)
				editMsg.ParseMode = "Markdown"
				editMsg.ReplyMarkup = &keyboard
				_, err = tg.Send(editMsg)
				if err != nil {
					log.Printf("Ошибка редактирования сообщения: %v", err)
				}
			}
		}
	}
}
