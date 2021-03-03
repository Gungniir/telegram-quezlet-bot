package telegram

import (
	"context"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/gungniir/telegram-quezlet-bot/database"
	"github.com/gungniir/telegram-quezlet-bot/models"
	log "github.com/sirupsen/logrus"
	"regexp"
	"strconv"
	"strings"
)

const groupKey = "groupKey"

func forGroup(ctx context.Context) *models.Group {
	return ctx.Value(groupKey).(*models.Group)
}

var (
	kbForNew = tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Создать свою группу"),
			tgbotapi.NewKeyboardButton("Присоединиться к группе"),
		),
	)
	kbForAuthed = tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Добавить модуль"),
			tgbotapi.NewKeyboardButton("Расписание повторений"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Выйти из группы"),
		),
	)
)

type TgServerConfig struct {
	Token string
}

type TgServer struct {
	Config       *TgServerConfig
	api          *tgbotapi.BotAPI
	stats        UserStatus
	userContexts UserContexts
	db           database.Database
	ticker       *Ticker
}

func (s *TgServer) ListenAndServe(db database.Database) error {
	api, err := tgbotapi.NewBotAPI(s.Config.Token)

	if err != nil {
		return err
	}

	s.api = api
	s.db = db

	s.ticker = new(Ticker)
	s.ticker.StartTicker(api, db)

	updates, err := api.GetUpdatesChan(tgbotapi.NewUpdate(0))

	if err != nil {
		return err
	}

	return s.listenUpdates(updates)
}

func (s *TgServer) listenUpdates(updates tgbotapi.UpdatesChannel) error {
	ctx := context.Background()
	for update := range updates {
		if update.Message != nil {
			group, err := s.db.GetUserGroup(ctx, update.Message.From.ID)

			if err != nil {
				log.WithError(err).Warn("Failed to get user group")
			} else {
				ctx = context.WithValue(ctx, groupKey, group)
			}

			_ = s.db.SetChatIDByUserID(ctx, update.Message.Chat.ID, update.Message.From.ID)
		} else if update.CallbackQuery != nil {
			group, err := s.db.GetUserGroup(ctx, update.CallbackQuery.From.ID)

			if err != nil {
				log.WithError(err).Warn("Failed to get user group")
			} else {
				ctx = context.WithValue(ctx, groupKey, group)
			}

			_ = s.db.SetChatIDByUserID(ctx, update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.From.ID)
		}

		if update.Message != nil && update.Message.IsCommand() {
			var err error

			switch update.Message.Command() {
			case "start":
				err = s.commandStart(ctx, update.Message)
			case "help":
				err = s.commandHelp(ctx, update.Message)
			case "quit":
				err = s.commandQuit(ctx, update.Message)
			case "cancel":
				err = s.commandCancel(ctx, update.Message)
			case "items":
				err = s.commandItems(ctx, update.Message)
			case "create_item":
				err = s.commandCreateItem(ctx, update.Message)
			case "tick":
				err = s.commandTick(ctx, update.Message)
			}

			if err != nil {
				return err
			}
		}
		if update.Message != nil && !update.Message.IsCommand() {
			var err error
			status := s.stats.Get(update.Message.From.ID)

			if status == UStatusUndefined {
				switch update.Message.Text {
				case "Создать свою группу":
					err = s.createGroupStart(ctx, update.Message)
				case "Присоединиться к группе":
					err = s.joinGroupStart(ctx, update.Message)
				case "Выйти из группы":
					err = s.commandQuit(ctx, update.Message)
				case "Расписание повторений":
					err = s.commandItems(ctx, update.Message)
				case "Добавить модуль":
					err = s.commandCreateItem(ctx, update.Message)
				default:
					err = s.defaultMessage(ctx, update.Message)
				}
			} else {
				switch s.stats.Get(update.Message.From.ID) {
				case UStatusCreateGroupSetPassword:
					err = s.createGroupSetPassword(ctx, update.Message)
				case UStatusJoinGroupCheckGroup:
					err = s.joinGroupCheckGroup(ctx, update.Message)
				case UStatusJoinGroupCheckPassword:
					err = s.joinGroupCheckPassword(ctx, update.Message)
				case UStatusCreateItemSetURL:
					err = s.createItemSetURL(ctx, update.Message)
				case UStatusCreateItemSetName:
					err = s.createItemSetName(ctx, update.Message)
				}
			}

			if err != nil {
				return err
			}
		}
		if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
			var err error

			switch strings.Split(update.CallbackQuery.Data, ":")[0] {
			case "SETOK":
				err = s.queryOk(ctx, update.CallbackQuery)
			}

			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Queries

func (s *TgServer) queryOk(ctx context.Context, query *tgbotapi.CallbackQuery) error {
	_, err := s.api.AnswerCallbackQuery(tgbotapi.NewCallback(query.ID, "Отлично!"))

	if err != nil {
		log.WithError(err).Warn("Failed to answer query")
		return nil
	}

	editText := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, query.Message.Text+"\nПовторили!")

	_, err = s.api.Send(editText)
	if err != nil {
		log.WithError(err).Warn("Failed to edit text")
		return nil
	}

	t := strings.Split(strings.Split(query.Data, ":")[1], ".")

	if len(t) < 2 {
		log.WithError(err).Warn("Failed parse. Expected 2 params")
		return nil
	}

	itemID, err := strconv.Atoi(t[0])

	if err != nil {
		log.WithError(err).Warn("Failed parse data")
		return nil
	}

	counter, err := strconv.Atoi(t[1])

	if err != nil {
		log.WithError(err).Warn("Failed parse data")
		return nil
	}

	err = s.db.NextItemByItemIDWithCheck(ctx, itemID, counter)

	if err != nil {
		log.WithError(err).Warn("Failed to next item")
		return nil
	}

	return nil
}

// Commands

func (s *TgServer) commandHelp(_ context.Context, msg *tgbotapi.Message) error {
	_, err := s.api.Send(tgbotapi.NewMessage(msg.Chat.ID,
		"Я напоминаю вам, каждый раз, когда приходит время освежить в памяти какие-нибудь карточки\n"+
			"• /help - Вывести данное сообщение",
	))

	return err
}

func (s *TgServer) commandStart(ctx context.Context, msg *tgbotapi.Message) error {
	var text string
	var kb tgbotapi.ReplyKeyboardMarkup
	group := forGroup(ctx)

	if group == nil {
		text = "Я напоминаю вам, каждый раз, когда приходит время освежить в памяти какие-нибудь карточки\n" +
			"Давайте начнём!"
		kb = kbForNew
	} else {
		text = "С возвращением! Вы находитесь в группе √" + strconv.Itoa(group.ID)
		kb = kbForAuthed
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, text)
	kb.OneTimeKeyboard = true

	m.ReplyMarkup = kb

	_, err := s.api.Send(m)

	return err
}

func (s *TgServer) commandCancel(ctx context.Context, msg *tgbotapi.Message) error {
	var kb tgbotapi.ReplyKeyboardMarkup
	group := forGroup(ctx)

	if group == nil {
		kb = kbForNew
	} else {
		kb = kbForAuthed
	}

	kb.OneTimeKeyboard = true

	m := tgbotapi.NewMessage(msg.Chat.ID,
		"Без вопросов",
	)

	m.ReplyMarkup = kb

	s.stats.Set(msg.From.ID, UStatusUndefined)

	_, err := s.api.Send(m)

	return err
}

func (s *TgServer) commandQuit(ctx context.Context, msg *tgbotapi.Message) error {
	kb := kbForNew
	kb.OneTimeKeyboard = true

	err := s.db.RemoveUserGroup(ctx, msg.From.ID)

	if err != nil {
		log.WithError(err).Warn("Failed to remove user from group")

		m := tgbotapi.NewMessage(msg.Chat.ID,
			"Не удалось выйти из группы, увы :(",
		)

		_, err = s.api.Send(m)
		return err
	}

	m := tgbotapi.NewMessage(msg.Chat.ID,
		"Вы вышли из группы",
	)

	m.ReplyMarkup = kb

	s.stats.Set(msg.From.ID, UStatusUndefined)

	_, err = s.api.Send(m)
	return err
}

func (s *TgServer) commandItems(ctx context.Context, msg *tgbotapi.Message) error {
	var kb tgbotapi.ReplyKeyboardMarkup
	var text string
	group := forGroup(ctx)

	if group == nil {
		kb = kbForNew
		text = "Вы не состоите в группе"
	} else {
		kb = kbForAuthed
		text = "*Расписание*\n"

		items, err := s.db.GetItemsByGroupID(ctx, group.ID)

		if err != nil {
			text = "Не удалось получить расписание"
		} else {
			for i, item := range items {
				text += fmt.Sprintf("\n%d. (%d.%d.%d) %s\nСсылка на модуль: [тыц](%s)",
					i+1, item.RepeatAt.Day(), item.RepeatAt.Month(), item.RepeatAt.Year(), item.Name, item.URL,
				)
			}
		}
	}

	kb.OneTimeKeyboard = true

	m := tgbotapi.NewMessage(msg.Chat.ID, text)
	m.DisableWebPagePreview = true
	m.ReplyMarkup = kb
	m.ParseMode = tgbotapi.ModeMarkdown

	_, err := s.api.Send(m)
	return err
}

func (s *TgServer) commandTick(_ context.Context, _ *tgbotapi.Message) error {
	s.ticker.tick()
	return nil
}

func (s *TgServer) commandCreateItem(ctx context.Context, msg *tgbotapi.Message) error {
	group := forGroup(ctx)
	m := tgbotapi.NewMessage(msg.Chat.ID, "")

	if group == nil {
		kb := kbForNew
		kb.OneTimeKeyboard = true
		m.Text = "Вы не состоите в группе"
		m.ReplyMarkup = kb
	} else {
		m.Text = "Новый модуль? Ок... Скиньте ссылку на него"
		s.stats.Set(msg.From.ID, UStatusCreateItemSetURL)
	}

	_, err := s.api.Send(m)
	return err
}

// CreateGroupFunctions

func (s *TgServer) createGroupStart(ctx context.Context, msg *tgbotapi.Message) error {
	var text string

	if forGroup(ctx) != nil {
		text = "Вы уже состоите в группе. Чтобы покинуть её введите /quit"
	} else {
		s.stats.Set(msg.From.ID, UStatusCreateGroupSetPassword)
		text = "Ок, придумайте пароль (как минимум 3 символа латиницей или цифрами)"
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, text)

	_, err := s.api.Send(m)

	return err
}

func (s *TgServer) createGroupSetPassword(_ context.Context, msg *tgbotapi.Message) error {
	password := msg.Text

	if !(*models.Group).CheckPassword(nil, password) {
		m := tgbotapi.NewMessage(msg.Chat.ID, "Недопустимый пароль, попробуйте другой")

		_, err := s.api.Send(m)
		return err
	}

	group, err := s.db.CreateGroup(context.Background(), (*models.Group).HashPassword(nil, password))

	if err != nil {
		log.WithError(err).Warn("Failed to create group")

		m := tgbotapi.NewMessage(msg.Chat.ID, "Не получилось создать группу, попробуйте еще раз")

		_, err := s.api.Send(m)
		return err
	}

	err = s.db.SetUserGroup(context.Background(), msg.From.ID, group.ID)

	if err != nil {
		log.WithError(err).Warn("Failed to attach user to group")

		m := tgbotapi.NewMessage(msg.Chat.ID, "Не получилось создать группу, попробуйте еще раз")

		_, err := s.api.Send(m)
		return err
	}

	m := tgbotapi.NewMessage(msg.Chat.ID,
		"Отлично, группа создана!\n"+
			"Вы можете пригласить в нее друзей по ID: "+strconv.Itoa(group.ID),
	)
	m.ReplyMarkup = kbForAuthed

	s.stats.Set(msg.From.ID, UStatusUndefined)

	_, err = s.api.Send(m)
	return err
}

// JoinGroupFunctions

func (s *TgServer) joinGroupStart(ctx context.Context, msg *tgbotapi.Message) error {
	var text string

	if forGroup(ctx) != nil {
		text = "Вы уже состоите в группе. Чтобы покинуть её введите /quit"
	} else {
		s.stats.Set(msg.From.ID, UStatusJoinGroupCheckGroup)
		text = "Ок, введите ID группы, к которой хотите присоединиться"
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, text)

	_, err := s.api.Send(m)

	return err
}

func (s *TgServer) joinGroupCheckGroup(ctx context.Context, msg *tgbotapi.Message) error {
	id, err := strconv.Atoi(msg.Text)

	if err != nil {
		m := tgbotapi.NewMessage(msg.Chat.ID, "Вы точно ввели число?")
		_, err := s.api.Send(m)

		return err
	}

	group, err := s.db.GetGroup(ctx, id)

	if err != nil {
		m := tgbotapi.NewMessage(msg.Chat.ID, "Не удалось проверить наличие группы, попробуйте ещё раз")
		_, err := s.api.Send(m)

		return err
	}

	if group == nil {
		m := tgbotapi.NewMessage(msg.Chat.ID, "Такой группы не существует, попробуйте ввести другой ID")
		_, err := s.api.Send(m)

		return err
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, "Хорошо, теперь введите пароль")
	s.stats.Set(msg.From.ID, UStatusJoinGroupCheckPassword)

	s.userContexts.Set(msg.From.ID, "JoinGroup_GroupID", strconv.Itoa(group.ID))

	_, err = s.api.Send(m)

	return err
}

func (s *TgServer) joinGroupCheckPassword(ctx context.Context, msg *tgbotapi.Message) error {
	password := msg.Text
	id, err := strconv.Atoi(s.userContexts.Get(msg.From.ID, "JoinGroup_GroupID"))

	if err != nil {
		m := tgbotapi.NewMessage(msg.Chat.ID, "Что-то пошло не так... Вернитесь в начало с помощью /cancel")
		_, err := s.api.Send(m)

		return err
	}

	if !(*models.Group).CheckPassword(nil, password) {
		m := tgbotapi.NewMessage(msg.Chat.ID, "Неверный формат пароля, попробуйте ещё раз")
		_, err := s.api.Send(m)

		return err
	}

	group, err := s.db.GetGroup(ctx, id)

	if err != nil {
		m := tgbotapi.NewMessage(msg.Chat.ID, "Не удалось проверить наличие группы, попробуйте ещё раз")
		_, err := s.api.Send(m)

		return err
	}

	if group == nil {
		m := tgbotapi.NewMessage(msg.Chat.ID, "Группа перестала существовать... Вернитесь в начало с помощью /cancel")
		_, err := s.api.Send(m)

		return err
	}

	if group.PasswordHash != group.HashPassword(password) {
		m := tgbotapi.NewMessage(msg.Chat.ID, "Неверный пароль, попробуйте ещё раз")
		_, err := s.api.Send(m)

		return err
	}

	err = s.db.SetUserGroup(ctx, msg.From.ID, group.ID)

	if err != nil {
		m := tgbotapi.NewMessage(msg.Chat.ID, "Не удалось добавить вас в группу, попробуйте ещё раз")
		_, err := s.api.Send(m)

		return err
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, "Добро пожаловать в группу √"+strconv.Itoa(group.ID))
	m.ReplyMarkup = kbForAuthed

	s.stats.Set(msg.From.ID, UStatusUndefined)

	_, err = s.api.Send(m)

	return err
}

// CreateItemFunctions

func (s *TgServer) createItemSetURL(ctx context.Context, msg *tgbotapi.Message) error {
	group := forGroup(ctx)
	m := tgbotapi.NewMessage(msg.Chat.ID, "")

	if group == nil {
		m.Text = "Вы не состоите в группе"
		_, err := s.api.Send(m)
		return err
	}

	url := msg.Text

	if !(*models.Item).CheckURL(nil, url) {
		m.Text = "Проверьте ссылку, мне кажется, что она неверная"
		_, err := s.api.Send(m)
		return err
	}

	s.userContexts.Set(msg.From.ID, "CreateItem_URL", url)
	s.stats.Set(msg.From.ID, UStatusCreateItemSetName)

	m.Text = "Окей, а теперь введите название модуля"
	_, err := s.api.Send(m)
	return err
}

func (s *TgServer) createItemSetName(ctx context.Context, msg *tgbotapi.Message) error {
	group := forGroup(ctx)
	m := tgbotapi.NewMessage(msg.Chat.ID, "")

	if group == nil {
		m.Text = "Вы не состоите в группе"
		_, err := s.api.Send(m)
		return err
	}

	name := msg.Text

	if !(*models.Item).CheckName(nil, name) {
		m.Text = "Ухх, плохое название, придумайте другое"
		_, err := s.api.Send(m)
		return err
	}

	url := s.userContexts.Get(msg.From.ID, "CreateItem_URL")

	if !(*models.Item).CheckURL(nil, url) {
		m.Text = "Что-то у меня амнезия... Я ссылку-то уде забыл... Давайте заново? Введите /cancel"
		_, err := s.api.Send(m)
		return err
	}

	item, err := s.db.CreateItem(ctx, group.ID, url, name)

	if err != nil {
		log.WithError(err).Error("Failed to create item")
		m.Text = "Тэкс... Я не смогу записать... Повторите, пожалуйста, еще раз..."
		_, err := s.api.Send(m)
		return err
	}

	s.stats.Set(msg.From.ID, UStatusUndefined)

	m.Text = fmt.Sprintf("Отлично! Карточка добавлена :)\nПовторим её %d.%d.%d", item.RepeatAt.Day(), item.RepeatAt.Month(), item.RepeatAt.Year())
	_, err = s.api.Send(m)
	return err
}

func (s *TgServer) createItemConfirm(ctx context.Context, msg *tgbotapi.Message) error {
	group := forGroup(ctx)
	m := tgbotapi.NewMessage(msg.Chat.ID, "")

	if group == nil {
		m.Text = "Вы не состоите в группе"
		_, err := s.api.Send(m)
		return err
	}

	name := msg.Text

	if !(*models.Item).CheckName(nil, name) {
		m.Text = "Ухх, плохое название, придумайте другое"
		_, err := s.api.Send(m)
		return err
	}

	url := s.userContexts.Get(msg.From.ID, "CreateItem_URL")

	if !(*models.Item).CheckURL(nil, url) {
		m.Text = "Что-то у меня амнезия... Я ссылку-то уде забыл... Давайте заново? Введите /cancel"
		_, err := s.api.Send(m)
		return err
	}

	item, err := s.db.CreateItem(ctx, group.ID, url, name)

	if err != nil {
		log.WithError(err).Error("Failed to create item")
		m.Text = "Тэкс... Я не смогу записать... Повторите, пожалуйста, еще раз..."
		_, err := s.api.Send(m)
		return err
	}

	s.stats.Set(msg.From.ID, UStatusUndefined)

	m.Text = fmt.Sprintf("Отлично! Карточка добавлена :)\nНазвание: %s\nСсылка: [тыц](%s)\nПовторим её %d.%d.%d", item.Name, item.URL, item.RepeatAt.Day(), item.RepeatAt.Month(), item.RepeatAt.Year())
	m.ParseMode = tgbotapi.ModeMarkdown
	m.ReplyMarkup = kbForAuthed

	_, err = s.api.Send(m)
	return err
}

func (s *TgServer) defaultMessage(ctx context.Context, msg *tgbotapi.Message) error {
	group := forGroup(ctx)

	newModuleRegex := regexp.MustCompile(`^(?:Я изучаю|Studying) ([\w\dА-Яа-я ():,.\-\\/&]{3,128}) (?:на|on) Quizlet: (http[s]?://(?:[a-zA-Z]|[0-9]|[$-_@.&+]|[!*(),]|(?:%[0-9a-fA-F][0-9a-fA-F]))+)$`)

	switch {
	case group != nil && newModuleRegex.MatchString(msg.Text):
		values := newModuleRegex.FindAllStringSubmatch(msg.Text, 1)
		name := values[0][1]
		url := values[0][2]

		m := tgbotapi.NewMessage(msg.Chat.ID, "")

		item, err := s.db.CreateItem(ctx, group.ID, url, name)

		if err != nil {
			log.WithError(err).Error("Failed to create item")
			m.Text = "Тэкс... Я не смогу записать... Повторите, пожалуйста, еще раз..."
			_, err := s.api.Send(m)
			return err
		}

		s.stats.Set(msg.From.ID, UStatusUndefined)

		m.Text = fmt.Sprintf("Отлично! Карточка добавлена :)\nНазвание: %s\nСсылка: [тыц](%s)\nПовторим её %d.%d.%d", item.Name, item.URL, item.RepeatAt.Day(), item.RepeatAt.Month(), item.RepeatAt.Year())
		m.ParseMode = tgbotapi.ModeMarkdown
		m.ReplyMarkup = kbForAuthed

		_, err = s.api.Send(m)
		return err
	default:
		m := tgbotapi.NewMessage(msg.Chat.ID, "Не понимаю, что вы имели в виду...")

		if group != nil {
			m.ReplyMarkup = kbForAuthed
		} else {
			m.ReplyMarkup = kbForNew
		}

		_, err := s.api.Send(m)

		if err != nil {
			log.WithError(err).Error("Failed to send default msg")
			return err
		}
	}

	return nil
}
