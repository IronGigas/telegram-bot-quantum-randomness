package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"telegram_bot_quantum_randomness/pkg/logger"
	qunatum "telegram_bot_quantum_randomness/pkg/qunatum_request"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Мапа для отслеживания состояния юзеров (ожидаем решения бота или idle)
var userStates = make(map[int64]string)
var stateMu sync.Mutex

const (
	StateIdle           = "IDLE"
	StateWaitingForList = "WAITING_FOR_LIST"
)

// Клавиатура бота
var numericKeyboard = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("🎲 Сделать выбор"),
		tgbotapi.NewKeyboardButton("ℹ️ Информация"),
	),
)

func main() {
	//запускаем отчистку кеша по таймеру
	qunatum.StartCacheCleaner()

	//инициализация бота
	botToken := "paster_your_token_here"
	if envToken := os.Getenv("BOT_TOKEN"); envToken != "" { //можно положить токен в переменную среды
		botToken = envToken
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = false
	log.Printf("Authorized on account %s", bot.Self.UserName)

	//конфигуриация апдейтов
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	//обрабатываем апдейты - основная логика бота
	for update := range updates {
		if update.Message == nil { //апдейты без сообщения игнорируем
			continue
		}

		chatID := update.Message.Chat.ID
		userName := update.Message.From.UserName
		text := update.Message.Text

		//пишем в лог
		logger.GetInstance().Log(chatID, userName, text, "INCOMONG")

		var msg tgbotapi.MessageConfig

		//команды и кнопки
		switch text {
		case "/start":
			msg = tgbotapi.NewMessage(chatID, "Привет! Я Квантовый Бот ⚛️.\nЯ помогу тебе выбрать вариант, используя настоящую квантовую случайность. Жми кнопку 'Сделать выбор'")
			msg.ReplyMarkup = numericKeyboard
			setState(chatID, StateIdle)

		case "ℹ️ Информация":
			infoText := "Этот бот использует API Австралийского национального университета (ANU).\n" +
				"Они измеряют квантовые флуктуации вакуума для генерации истинно случайных чисел.\n" +
				"В отличие от обычных компьютерных алгоритмов, эти числа невозможно предсказать.\n" +
				"Вот что происходит под капотом...\n" +
				"Бот подключается к реальному квантовому генератору случайных чисел, который разработан в Австралийском национальном университете (ANU).\n" +
				"Он использует фундаментальную квантовую неопределённость для получения истинно случайных (не псевдослучайных) чисел.\n" +
				"Даже в идеальном вакууме (где нет атомов) существуют квантовые флуктуации - кратковременные случайные всплески (энергии) электромагнитного поля.\n" +
				"\n" +
				"Схема получения случайных числе следующая:\n" +
				"1) Работает лазер, он даёт стабильный 'эталонный' свет (когерентное поле).\n" +
				"\n" +
				"2) Луч лазера направляют на делитель луча (beam splitter), который имеет два входа и два выхода.\n" +
				"На один вход подается свет от лазера, а на второй вход ничего не подаётся - там остается вакуум, но в квантовом мире эта пустота содержит случайные, непредсказуемые колебания (виртуальные фотоны).\n" +
				"\n" +
				"3) Делитель смешивает лазерный свет с этими колебаниями и выдаёт два выходных луча: каждый из них содержит часть стабильного лазерного света плюс случайный `шум` от вакуума.\n" +
				"\n" +
				"4) После делителя два выхода падают на балансный гомодинный приёмник: два фотодиода измеряют интенсивности выходных пучков, и их потоки (пропорциональные числу фотонов) вычитаются.\n" +
				"Это делается потому, что вакуумные флуктуации на столько слабы, что чтобы их можно было детектировать лазер используется как усилитель, который перекрывает сам себя.\n" +
				"Это устраняет классический `шум` и сигнал от лазера, оставляя только квантовые возмущения вакуума - по сути произвольные колебания квадратур поля.\n" +
				"\n" +
				"5) Этот результат представляет собой аналоговый сигнал, который цифруют через аналогово-цифровой преобразователь в дискретные значения (байты).\n" +
				"Каждый отсчёт - число, пропорциональное измеренной квантовой флуктуации в момент измерения.\n" +
				"\n" +
				"Итог: цифровая последовательность, коренящаяся в фундаментальной квантовой неопределённости вакуума - её нельзя предсказать заранее никаким классическим способом.\n" +
				"Эта последовательность используется для выбора рандомного варианта из списка.\n" +
				"\n" +
				"Чтобы убедиться что всё честно:\n" +
				"Сайт проекта который предоставляет API - https://qrng.anu.edu.au\n" +
				"Репозитарий Github с исходным кодом бота - https://github.com/IronGigas/telegram-bot-quantum-randomness"
			msg = tgbotapi.NewMessage(chatID, infoText)
			msg.ReplyMarkup = numericKeyboard
			setState(chatID, StateIdle)

		case "🎲 Сделать выбор":
			msg = tgbotapi.NewMessage(chatID, "Отправь мне список вариантов через пробел или запятую.\nНапример: `Дима Илья Лёша` или `Кино, Парк, Сон`")
			msg.ParseMode = "Markdown"
			setState(chatID, StateWaitingForList)

		default:
			//на основе состояния работаем с сообщениями чата
			currentState := getState(chatID)

			if currentState == StateWaitingForList {
				//отправляем сообщение имитации бурной деятельности
				magicMsg := tgbotapi.NewMessage(chatID, "🪄 *Вжух-вжух...* связываемся с квантовым миром...")
				magicMsg.ParseMode = "Markdown"
				bot.Send(magicMsg)

				//притворяемся, что работаем 2 секунды
				time.Sleep(2 * time.Second)

				//обрабатываем ипут пользователя
				options := parseOptions(text)

				if len(options) < 2 {
					msg = tgbotapi.NewMessage(chatID, "Пожалуйста, введите хотя бы два варианта.")
				} else {
					//делаем квантовый выбор
					choice, err := qunatum.GetQuantumChoice(options)
					if err != nil {
						//если не сработало, то логика должна уйти в фоллбек на обычный crypto/rand, но если уж полный отказ бота то выводим сообщение
						msg = tgbotapi.NewMessage(chatID, "Произошла магия, но что-то пошло не так. Попробуйте еще раз.")
						log.Printf("Error getting choice: %v", err)

					} else {
						msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("⚛️ Квантовая вселенная выбрала:\n\n✨ *%s* ✨", choice))
						msg.ParseMode = "Markdown"
					}
				}
				//сбрасываем state
				setState(chatID, StateIdle)
				msg.ReplyMarkup = numericKeyboard

			} else {
				msg = tgbotapi.NewMessage(chatID, "Нажми кнопку '🎲 Сделать выбор', чтобы сделать рандомный выбор. Или кнопку 'ℹ️ Информация', чтобы узнать как это всё работает.")
				msg.ReplyMarkup = numericKeyboard
			}
		}

		if _, err := bot.Send(msg); err != nil {
			log.Printf("Ошибка отправки сообщения: %v", err)
		}

		logger.GetInstance().Log(chatID, "BOT", msg.Text, "OUTGOING")

	}
}

// функция для парсинга входящих сообщений через запятые или через пробелы
func parseOptions(input string) []string {
	var parts []string
	if strings.Contains(input, ",") {
		parts = strings.Split(input, ",")
	} else {
		parts = strings.Split(input, " ")
	}

	var clean []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	return clean
}

// функция для потокобезопасного управления чатом
func setState(chatID int64, state string) {
	stateMu.Lock()
	defer stateMu.Unlock()
	userStates[chatID] = state
}

// функция для потокобезопасного управления чатом
func getState(chatID int64) string {
	stateMu.Lock()
	defer stateMu.Unlock()
	if val, ok := userStates[chatID]; ok {
		return val
	}
	return StateIdle
}
