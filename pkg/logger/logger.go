package logger

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	logFileName = "bot_dialog.log"
	maxLogSize = 1024 * 1024 * 1024 //задаем максимальный размер лог файла 1гб
)

type FileLogger struct {
	mu sync.Mutex
}

var instance *FileLogger
var once sync.Once

// Функция для получения инстанса чата
func GetInstance() *FileLogger {
	once.Do(func() {
		instance = &FileLogger{}
	})
	return instance
}

// Метод для записи сообщения в файл лога
func (l *FileLogger) Log(chatID int64, username, text string, direction string) {
	//вешаем лок на файл
	l.mu.Lock()
	defer l.mu.Unlock()

	//проверка размера лог файла и транкейт оного, если нужно
	info, err := os.Stat(logFileName)
	if err == nil && info.Size() >= maxLogSize {
		_ = os.Truncate(logFileName, 0)
	}

	f, err := os.OpenFile(logFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opnening log file:", err)
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] [%s] [%s:%d] %s\n", timestamp, direction, username, chatID, text)

	if _, err := f.WriteString(logLine); err != nil {
		fmt.Println("Error writing to log:", err)
	}

}