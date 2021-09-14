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
	"time"
)

var newModuleRegex = regexp.MustCompile(`^(?:Я изучаю|Studying) ([\w\dА-Яа-я ():,.\-\\/&]{3,128}) (?:на|on) Quizlet: (http[s]?://(?:[a-zA-Z]|[0-9]|[$-_@.&+]|[!*(),]|(?:%[0-9a-fA-F][0-9a-fA-F]))+)$`)

const (
	groupKey                    = "groupKey"
	msgYouDoNotBelongToAnyGroup = "Вы не состоите в группе"

	butCreateNewGroup = "Создать свою группу"
	butJoinGroup      = "Присоединиться к группе"
	butAddModule      = "Добавить модуль"
	butGetSchedule    = "Расписание повторений"
	butLeaveGroup     = "Покинуть группу"
)

func forGroup(ctx context.Context) []*models.Group {
	return ctx.Value(groupKey).([]*models.Group)
}

var (
	kbForNew = tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(butCreateNewGroup),
			tgbotapi.NewKeyboardButton(butJoinGroup),
		),
	)
	kbForAuthed = tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(butAddModule),
			tgbotapi.NewKeyboardButton(butGetSchedule),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(butCreateNewGroup),
			tgbotapi.NewKeyboardButton(butJoinGroup),
			tgbotapi.NewKeyboardButton(butLeaveGroup),
		),
	)
)

type TgServerConfig struct {
	Token    string
	Timezone *time.Location
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

	wbInfo, err := api.GetWebhookInfo()

	if wbInfo.IsSet() {
		log.Warn("Webhook is set")
		log.Warn(wbInfo.URL)

		api.RemoveWebhook()
	}

	if err != nil {
		return err
	}

	s.api = api
	s.db = db

	s.ticker = new(Ticker)
	s.ticker.timezone = s.Config.Timezone
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
			groups, err := s.db.GetUserGroups(ctx, update.Message.From.ID)

			if err != nil {
				log.WithError(err).Warn("Failed to get user group")
			} else {
				ctx = context.WithValue(ctx, groupKey, groups)
			}

			_ = s.db.SetChatIDByUserID(ctx, update.Message.Chat.ID, update.Message.From.ID)
		} else if update.CallbackQuery != nil {
			groups, err := s.db.GetUserGroups(ctx, update.CallbackQuery.From.ID)

			if err != nil {
				log.WithError(err).Warn("Failed to get user group")
			} else {
				ctx = context.WithValue(ctx, groupKey, groups)
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
			case "time":
				err = s.commandTime(ctx, update.Message)
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
				case butCreateNewGroup:
					err = s.createGroupStart(ctx, update.Message)
				case butJoinGroup:
					err = s.joinGroupStart(ctx, update.Message)
				case butLeaveGroup:
					err = s.commandQuit(ctx, update.Message)
				case butGetSchedule:
					err = s.commandItems(ctx, update.Message)
				case butAddModule:
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
				case UStatusCreateItemChoseGroup:
					err = s.createItemChoseGroup(ctx, update.Message)
				case UStatusCreateItemSetName:
					err = s.createItemSetName(ctx, update.Message)
				case UStatusCreateFullItemChoseGroup:
					err = s.createFullItemChoseGroup(ctx, update.Message)
				case UStatusLeaveGroupChoseGroup:
					err = s.leaveGroupChoseGroup(ctx, update.Message)
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

	err = s.db.ProlongByItemIDWithCheck(ctx, itemID, counter)

	if err != nil {
		log.WithError(err).Warn("Failed to next item")
		return nil
	}

	return nil
}

// Commands

func (s *TgServer) commandHelp(_ context.Context, msg *tgbotapi.Message) error {
	m := tgbotapi.NewMessage(msg.Chat.ID,
		"Я напоминаю вам, каждый раз, когда приходит время освежить в памяти какие-нибудь карточки\n"+
			"• /help - Вывести данное сообщение\n"+
			"• /cancel - Сбросить состояние, вернуться в главное меню\n",
	)

	kb := kbForAuthed
	kb.OneTimeKeyboard = true

	m.ReplyMarkup = kb

	_, err := s.api.Send(m)

	return err
}

func (s *TgServer) commandStart(ctx context.Context, msg *tgbotapi.Message) error {
	var text string
	var kb tgbotapi.ReplyKeyboardMarkup
	groups := forGroup(ctx)

	if groups == nil {
		text = "Я напоминаю вам, каждый раз, когда приходит время освежить в памяти какие-нибудь карточки\n" +
			"Давайте начнём!"
		kb = kbForNew
	} else if len(groups) == 1 {
		text = "С возвращением! Вы находитесь в группе √" + strconv.Itoa(groups[0].ID)
		kb = kbForAuthed
	} else {
		text = "С возвращением! Вы находитесь в группах √"

		for _, group := range groups {
			text += fmt.Sprintf("%d, ", group.ID)
		}

		text = text[:len(text)-2] // Удаляем лишний пробел и запятую

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
	groups := forGroup(ctx)

	if len(groups) == 1 {
		kb := kbForNew
		kb.OneTimeKeyboard = true

		err := s.db.RemoveUserFromGroup(ctx, msg.From.ID, groups[0].ID)

		if err != nil {
			log.WithError(err).Warn("Failed to remove user from group")

			m := tgbotapi.NewMessage(msg.Chat.ID,
				"Не удалось выйти из группы, увы :(",
			)

			_, err = s.api.Send(m)
			return err
		}

		m := tgbotapi.NewMessage(msg.Chat.ID,
			fmt.Sprintf("Вы вышли из группы √%d", groups[0].ID),
		)

		m.ReplyMarkup = kb

		s.stats.Set(msg.From.ID, UStatusUndefined)

		_, err = s.api.Send(m)
		return err
	} else if len(groups) == 0 {
		kb := kbForNew
		kb.OneTimeKeyboard = true

		m := tgbotapi.NewMessage(msg.Chat.ID, "Вы не находитесь в группе")
		m.ReplyMarkup = kb

		s.stats.Set(msg.From.ID, UStatusUndefined)

		_, err := s.api.Send(m)
		return err
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, "Выберите группу, из которой хотите выйти")

	_, err := s.api.Send(m)

	if err != nil {
		return err
	}

	m.Text = "Вы на ходитесь в группах √"

	for _, group := range groups {
		m.Text += fmt.Sprintf("%d, ", group.ID)
	}

	m.Text = m.Text[:len(m.Text)-2] // Убираем лишний пробел и запятую
	_, err = s.api.Send(m)

	s.stats.Set(msg.From.ID, UStatusLeaveGroupChoseGroup)

	return err
}

func (s *TgServer) commandItems(ctx context.Context, msg *tgbotapi.Message) error {
	var kb tgbotapi.ReplyKeyboardMarkup
	var text string
	groups := forGroup(ctx)

	if groups == nil {
		kb = kbForNew
		text = msgYouDoNotBelongToAnyGroup
	} else {
		kb = kbForAuthed

		for _, group := range groups {
			text += fmt.Sprintf("\n\n*Расписание группы √%d*\n", group.ID)

			items, err := s.db.GetItemsByGroupID(ctx, group.ID)

			if err != nil {
				text = "Не удалось получить расписание"
			} else {
				for i, item := range items {
					text += fmt.Sprintf("\n%d. (%02d.%02d.%d) %s\nСсылка на модуль: [тыц](%s)",
						i+1, item.RepeatAt.Day(), item.RepeatAt.Month(), item.RepeatAt.Year(), item.Name, item.URL,
					)
				}
			}
		}

		text = text[2:] // Убираем начальный \n
	}

	kb.OneTimeKeyboard = true

	m := tgbotapi.NewMessage(msg.Chat.ID, text)
	m.DisableWebPagePreview = true
	m.ReplyMarkup = kb
	m.ParseMode = tgbotapi.ModeMarkdown

	_, err := s.api.Send(m)
	return err
}

func (s *TgServer) commandTick(_ context.Context, msg *tgbotapi.Message) error {
	s.ticker.tick()

	m := tgbotapi.NewMessage(msg.Chat.ID, "Успешный тик")

	kb := kbForAuthed
	kb.OneTimeKeyboard = true

	m.ReplyMarkup = kb

	_, err := s.api.Send(m)
	return err
}

func (s *TgServer) commandTime(ctx context.Context, msg *tgbotapi.Message) error {
	nowDB, err := s.db.GetDate(ctx)

	if err != nil {
		return err
	}

	nowApp := time.Now().In(s.Config.Timezone)

	m := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("Время в приложении: %s\nВремя в базе данных: %s", nowApp.String(), nowDB.String()))

	kb := kbForAuthed
	kb.OneTimeKeyboard = true

	m.ReplyMarkup = kb

	_, err = s.api.Send(m)
	return err
}

func (s *TgServer) commandCreateItem(ctx context.Context, msg *tgbotapi.Message) error {
	group := forGroup(ctx)
	m := tgbotapi.NewMessage(msg.Chat.ID, "")

	if group == nil {
		kb := kbForNew
		kb.OneTimeKeyboard = true
		m.Text = msgYouDoNotBelongToAnyGroup
		m.ReplyMarkup = kb
	} else if len(group) == 1 {
		m.Text = "Новый модуль? Ок... Скиньте ссылку на него"

		kb := kbForAuthed
		kb.OneTimeKeyboard = true

		m.ReplyMarkup = kb
		s.stats.Set(msg.From.ID, UStatusCreateItemSetURL)
	} else {
		m.Text = "Новый модуль? Ок... В какую группу вы хотите его добавить?"

		kb := kbForAuthed
		kb.OneTimeKeyboard = true

		m.ReplyMarkup = kb
		s.stats.Set(msg.From.ID, UStatusCreateItemChoseGroup)
	}

	_, err := s.api.Send(m)
	return err
}

// CreateGroupFunctions

func (s *TgServer) createGroupStart(ctx context.Context, msg *tgbotapi.Message) error {
	var text string

	groups := forGroup(ctx)

	if groups != nil {
		text = "Напоминаю, что вы состоите в "

		if len(groups) > 1 {
			text += "группах"
		} else {
			text += "группе"
		}

		text += " √"

		for _, group := range groups {
			text += fmt.Sprintf("%d, ", group.ID)
		}

		text = text[:len(text)-2] // Удаляем ненужную последнюю запятую и пробел

		m := tgbotapi.NewMessage(msg.Chat.ID, text)

		_, err := s.api.Send(m)

		if err != nil {
			return err
		}
	}

	s.stats.Set(msg.From.ID, UStatusCreateGroupSetPassword)

	text = "Придумайте пароль (как минимум 3 символа латиницей или цифрами)"

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

	err = s.db.AddUserToGroup(context.Background(), msg.From.ID, group.ID)

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

	kb := kbForAuthed
	kb.OneTimeKeyboard = true

	m.ReplyMarkup = kb

	s.stats.Set(msg.From.ID, UStatusUndefined)

	_, err = s.api.Send(m)
	return err
}

// JoinGroupFunctions

func (s *TgServer) joinGroupStart(ctx context.Context, msg *tgbotapi.Message) error {
	var text string

	groups := forGroup(ctx)

	if groups != nil {
		text = "Напоминаю, что вы состоите в "

		if len(groups) > 1 {
			text += "группах"
		} else {
			text += "группе"
		}

		text += " √"

		for _, group := range groups {
			text += fmt.Sprintf("%d, ", group.ID)
		}

		text = text[:len(text)-2] // Удаляем ненужную последнюю запятую и пробел

		m := tgbotapi.NewMessage(msg.Chat.ID, text)

		_, err := s.api.Send(m)

		if err != nil {
			return err
		}
	}

	s.stats.Set(msg.From.ID, UStatusJoinGroupCheckGroup)
	text = "Введите ID группы, к которой хотите присоединиться"

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

	err = s.db.AddUserToGroup(ctx, msg.From.ID, group.ID)

	if err != nil {
		m := tgbotapi.NewMessage(msg.Chat.ID, "Не удалось добавить вас в группу, попробуйте ещё раз")
		_, err := s.api.Send(m)

		return err
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, "Добро пожаловать в группу √"+strconv.Itoa(group.ID))

	kb := kbForAuthed
	kb.OneTimeKeyboard = true

	m.ReplyMarkup = kb

	s.stats.Set(msg.From.ID, UStatusUndefined)

	_, err = s.api.Send(m)

	return err
}

// CreateItemFunctions

func (s *TgServer) createItemChoseGroup(ctx context.Context, msg *tgbotapi.Message) error {
	groups := forGroup(ctx)
	m := tgbotapi.NewMessage(msg.Chat.ID, "")

	if groups == nil {
		m.Text = msgYouDoNotBelongToAnyGroup
		_, err := s.api.Send(m)
		return err
	} else if len(groups) == 1 {
		s.userContexts.Set(msg.From.ID, "CreateItem_Group", strconv.Itoa(groups[0].ID))
		m.Text = fmt.Sprintf("Выбрана группа √%d", groups[0].ID)
		_, err := s.api.Send(m)
		if err != nil {
			return err
		}
		m.Text = "А теперь скиньте ссылку на модуль"
		_, err = s.api.Send(m)
		s.stats.Set(msg.From.ID, UStatusCreateItemSetURL)
		return err
	}

	groupID, err := strconv.Atoi(msg.Text)

	if err != nil {
		m.Text = "Вы уверены, что ввели число без всяких знаков? Повторите, пожалуйста, ещё раз"
		_, err = s.api.Send(m)
		return err
	}

	var allowed bool

	for _, group := range groups {
		if group.ID == groupID {
			allowed = true
			break
		}
	}

	if !allowed {
		m.Text = fmt.Sprintf("Вы не входите в группу √%d", groupID)
		_, err = s.api.Send(m)
		return err
	}

	s.userContexts.Set(msg.From.ID, "CreateItem_Group", strconv.Itoa(groupID))
	s.stats.Set(msg.From.ID, UStatusCreateItemSetURL)

	m.Text = "Отлично! А теперь скиньте ссылку на модуль"
	_, err = s.api.Send(m)

	return err
}

func (s *TgServer) createItemSetURL(ctx context.Context, msg *tgbotapi.Message) error {
	group := forGroup(ctx)
	m := tgbotapi.NewMessage(msg.Chat.ID, "")

	if group == nil {
		m.Text = msgYouDoNotBelongToAnyGroup
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
	groups := forGroup(ctx)
	m := tgbotapi.NewMessage(msg.Chat.ID, "")

	if groups == nil {
		m.Text = msgYouDoNotBelongToAnyGroup
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
		m.Text = "Что-то у меня амнезия... Я ссылку-то уже забыл... Давайте заново? Введите /cancel"
		_, err := s.api.Send(m)
		return err
	}

	rawGroupID := s.userContexts.Get(msg.From.ID, "CreateItem_Group")

	if len(groups) > 1 && rawGroupID == "" {
		m.Text = "Что-то у меня амнезия... Я выбранную группу уже забыл... Давайте заново? Введите /cancel"
		_, err := s.api.Send(m)
		return err
	}

	var item *models.Item
	var err error

	if len(groups) > 1 {
		groupID, _ := strconv.Atoi(rawGroupID)
		item, err = s.db.CreateItem(ctx, groupID, url, name)
	} else {
		item, err = s.db.CreateItem(ctx, groups[0].ID, url, name)
	}

	if err != nil {
		log.WithError(err).Error("Failed to create item")
		m.Text = "Тэкс... Я не смогу записать... Повторите, пожалуйста, еще раз..."
		_, err := s.api.Send(m)
		return err
	}

	s.stats.Set(msg.From.ID, UStatusUndefined)

	kb := kbForAuthed
	kb.OneTimeKeyboard = true

	m.ReplyMarkup = kb

	m.Text = fmt.Sprintf("Отлично! Карточка добавлена :)\nПовторим её %02d.%02d.%d", item.RepeatAt.Day(), item.RepeatAt.Month(), item.RepeatAt.Year())
	_, err = s.api.Send(m)
	return err
}

// create full item functions

func (s *TgServer) createFullItemChoseGroup(ctx context.Context, msg *tgbotapi.Message) error {
	groups := forGroup(ctx)
	m := tgbotapi.NewMessage(msg.Chat.ID, "")

	if groups == nil {
		m.Text = msgYouDoNotBelongToAnyGroup
		_, err := s.api.Send(m)
		return err
	} else if len(groups) == 1 {
		s.userContexts.Set(msg.From.ID, "CreateFullItem_Group", strconv.Itoa(groups[0].ID))
		m.Text = fmt.Sprintf("Выбрана группа √%d", groups[0].ID)
		_, err := s.api.Send(m)
		if err != nil {
			return err
		}
		s.stats.Set(msg.From.ID, UStatusUndefined)
		return s.createFullItemProcess(ctx, msg)
	}

	groupID, err := strconv.Atoi(msg.Text)

	if err != nil {
		m.Text = "Вы уверены, что ввели число без всяких знаков? Повторите, пожалуйста, ещё раз"
		_, err = s.api.Send(m)
		return err
	}

	var allowed bool

	for _, group := range groups {
		if group.ID == groupID {
			allowed = true
			break
		}
	}

	if !allowed {
		m.Text = fmt.Sprintf("Вы не входите в группу √%d", groupID)
		_, err = s.api.Send(m)
		return err
	}

	s.userContexts.Set(msg.From.ID, "CreateFullItem_Group", strconv.Itoa(groupID))
	s.stats.Set(msg.From.ID, UStatusUndefined)

	return s.createFullItemProcess(ctx, msg)
}

func (s *TgServer) createFullItemProcess(ctx context.Context, msg *tgbotapi.Message) error {
	rawModule := s.userContexts.Get(msg.From.ID, "CreateFullItem_Module")
	rawGroupID := s.userContexts.Get(msg.From.ID, "CreateFullItem_Group")

	groupID, _ := strconv.Atoi(rawGroupID)

	values := newModuleRegex.FindAllStringSubmatch(rawModule, 1)
	name := values[0][1]
	url := values[0][2]

	m := tgbotapi.NewMessage(msg.Chat.ID, "")

	item, err := s.db.CreateItem(ctx, groupID, url, name)

	if err != nil {
		log.WithError(err).Error("Failed to create item")
		m.Text = "Тэкс... Я не смогу записать... Повторите, пожалуйста, еще раз..."
		_, err := s.api.Send(m)
		return err
	}

	s.stats.Set(msg.From.ID, UStatusUndefined)

	m.Text = fmt.Sprintf("Отлично! Карточка добавлена в группу √%d :)\nНазвание: %s\nСсылка: [тыц](%s)\nПовторим её %02d.%02d.%d", groupID, item.Name, item.URL, item.RepeatAt.Day(), item.RepeatAt.Month(), item.RepeatAt.Year())
	m.ParseMode = tgbotapi.ModeMarkdown
	m.ReplyMarkup = kbForAuthed

	_, err = s.api.Send(m)
	return err
}

// leave group functions

func (s *TgServer) leaveGroupChoseGroup(ctx context.Context, msg *tgbotapi.Message) error {
	kb := kbForAuthed
	kb.OneTimeKeyboard = true

	groups := forGroup(ctx)

	m := tgbotapi.NewMessage(msg.Chat.ID, "")

	if groups == nil {
		m.Text = msgYouDoNotBelongToAnyGroup
		_, err := s.api.Send(m)
		return err
	} else if len(groups) == 1 {
		m.Text = fmt.Sprintf("Выбрана группа √%d", groups[0].ID)
		_, err := s.api.Send(m)
		if err != nil {
			return err
		}

		err = s.db.RemoveUserFromGroup(ctx, msg.From.ID, groups[0].ID)
		if err != nil {
			return err
		}
		if err != nil {
			m.Text = "Произошла неизвестная ошибка при выходе из группы, попробуйте ещё раз"
			_, err = s.api.Send(m)
			return err
		}

		s.stats.Set(msg.From.ID, UStatusUndefined)

		m.Text = fmt.Sprintf("Вы вышли из группы √%d", groups[0].ID)
		m.ReplyMarkup = kb
		_, err = s.api.Send(m)
		return err
	}

	groupID, err := strconv.Atoi(msg.Text)

	if err != nil {
		m.Text = "Вы уверены, что ввели число без всяких знаков? Повторите, пожалуйста, ещё раз"
		_, err = s.api.Send(m)
		return err
	}

	var allowed bool

	for _, group := range groups {
		if group.ID == groupID {
			allowed = true
			break
		}
	}

	if !allowed {
		m.Text = fmt.Sprintf("Вы не входите в группу √%d", groupID)
		_, err = s.api.Send(m)
		return err
	}

	err = s.db.RemoveUserFromGroup(ctx, msg.From.ID, groupID)

	if err != nil {
		m.Text = "Произошла неизвестная ошибка при выходе из группы, попробуйте ещё раз"
		_, err = s.api.Send(m)
		return err
	}

	s.stats.Set(msg.From.ID, UStatusUndefined)

	m.Text = fmt.Sprintf("Вы вйшли из группы √%d", groupID)
	m.ReplyMarkup = kb
	_, err = s.api.Send(m)
	return err
}

// default

func (s *TgServer) defaultMessage(ctx context.Context, msg *tgbotapi.Message) error {
	groups := forGroup(ctx)

	switch {
	case len(groups) == 1 && newModuleRegex.MatchString(msg.Text):
		s.userContexts.Set(msg.From.ID, "CreateFullItem_Module", msg.Text)
		s.userContexts.Set(msg.From.ID, "CreateFullItem_Group", strconv.Itoa(groups[0].ID))

		return s.createFullItemProcess(ctx, msg)
	case len(groups) > 1 && newModuleRegex.MatchString(msg.Text):
		s.userContexts.Set(msg.From.ID, "CreateFullItem_Module", msg.Text)

		m := tgbotapi.NewMessage(msg.Chat.ID, "")

		m.Text = "Введите номер группы, в которую хотите добавить эту карточку"
		_, err := s.api.Send(m)

		if err != nil {
			return err
		}

		m.Text = "Вы находитесь в группах √"

		for _, group := range groups {
			m.Text += fmt.Sprintf("%d, ", group.ID)
		}

		m.Text = m.Text[:len(m.Text)-2] // Удаляем лишнюю запятую и пробел

		_, err = s.api.Send(m)
		if err != nil {
			return err
		}

		s.stats.Set(msg.From.ID, UStatusCreateFullItemChoseGroup)
		return nil
	default:
		m := tgbotapi.NewMessage(msg.Chat.ID, "Не понимаю, что вы имели в виду...")

		if groups != nil {
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
