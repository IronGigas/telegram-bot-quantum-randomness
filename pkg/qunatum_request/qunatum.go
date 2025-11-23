package qunatum

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"sync"
	"time"
)

// структура ответа от сервиса
type AnuResp struct {
	Type   string `json:"type"`
	Length int    `json:"length"`
	Data   []int  `json:"data"`
}

// кеш с байтами, где мы храним ответ от сервиса
type byteCache struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

// переменная с кешем, с заданным лимитом
var cache = &byteCache{limit: 4096}
// переменные для Circuit Breaker (предохранитель API)
var apiBlockedUntil time.Time
var apiBlockMu sync.RWMutex

// метод для полечения одной цифры из кеша
func (c *byteCache) getOne() (byte, bool) {
	c.mu.Lock() //на всякий случай лочик кеш, если запросов будет очень много
	defer c.mu.Unlock()
	if len(c.buf) == 0 { //проверяем, что кеш не пустой
		return 0, false
	}
	b := c.buf[0]     //получем первое значение из кеша
	c.buf = c.buf[1:] //сдвигаем первое значение вправо, чтобы на следующем запросе была следуюшая цифра
	return b, true
}

// метод для того, чтобы положить в кеш байты от ANU
func (c *byteCache) putMany(bs []byte) {
	c.mu.Lock() //на всякий случай лочик кеш, пока ведем с ним операции
	defer c.mu.Unlock()
	c.buf = append(c.buf, bs...)             //кладем в буфер байты
	if c.limit > 0 && len(c.buf) > c.limit { //если байтов больше чем лимит, то оберзаем кеш
		c.buf = c.buf[:c.limit]
	}
}

// метод для очистки кеша с квантовыми байтами
func (c *byteCache) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = []byte{} //обнуляем буфер
	// fmt.Println("DEBUG: Quantum cache flushed") // раскоментить для дебага
}

// функция для очистки кеша по таймеру, запукает отдельную горутину для осчета времени
func StartCacheCleaner() {
	ticker := time.NewTicker(2 * time.Minute)  //каждые 2 минуты чистим кеш
	go func() {
		for range ticker.C {
			cache.flush()
		}
	}()
}

// функция для получения квантовых рандомных чисел (байт) от ANU
func getANUBytes(n int) ([]byte, error) {
	//проверка предохранителя - если API заблокирован (например спамом), сразу возвращаем ошибку
	apiBlockMu.RLock()
	if time.Now().Before(apiBlockedUntil) {
		apiBlockMu.RUnlock()
		return nil, errors.New("API временно заблокирован (circuit breaker)")
	}
	apiBlockMu.RUnlock()

	if n < 1 || n > 1024 { //проверка длины запроса
		return nil, errors.New("ANU request must be 1..1024 bytes")
	}

	url := fmt.Sprintf("https://qrng.anu.edu.au/API/jsonI.php?length=%d&type=uint8", n)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //конекст чтобы подождать ответа 5 секунд
	defer cancel()

	tr := &http.Transport{ //транспорт для клиента
		DialContext:         (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		TLSHandshakeTimeout: 3 * time.Second,
	}
	client := &http.Client{Transport: tr}

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil) //GET метод с контекстом
	resp, err := client.Do(req)                                //получаем респанс от сервиса
	if err != nil {
		blockAPI()  //блокируем API при ошибке сети
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 { //если код респонса не 200, то есть не ок, возвращаем что ответл сервис
		blockAPI()   //блокируем API при ошибке статуса (например 500)
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ANU status %d: %s", resp.StatusCode, string(body))
	}

	var r AnuResp                                                 //переменная для квантовых байт
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil { //проверка, что ответ корретный по структуре
		return nil, err
	}
	if len(r.Data) < n {
		return nil, errors.New("ANU returned insufficient data") //проверка длины ответа
	}

	out := make([]byte, n)
	for i := range out { //записываем данные в итоговый слайс
		out[i] = byte(r.Data[i])
	}

	return out, nil
}

// функция для блокировки запросов к API на 1 минуту при ошибках, чтобы бот не зависал
func blockAPI() {
	apiBlockMu.Lock()
	defer apiBlockMu.Unlock()
	apiBlockedUntil = time.Now().Add(1 * time.Minute)
	// fmt.Println("DEBUG: ANU API blocked for 1 minute due to error")
} 

// функция для опроса всех источников с данными, чтобы получить один байт
func getByteFromAnySource() (byte, error) {
	//получаем один байт из кеша
	if b, ok := cache.getOne(); ok {
		fmt.Println("DEBUG: получили байт из кеша")
		return b, nil
	}

	//запрашиваем у ANU 64 рандомных байта, можно по желанию больше. Каждый байт = одно случайное число в диапазоне 0–255.
	if bs, err := getANUBytes(64); err == nil {
		cache.putMany(bs[1:]) //записываем в кеш со сдвигом в один байт, так как его получаем ниже
		fmt.Println("DEBUG: получили байты от ANU")
		return bs[0], nil
	} else {
		fmt.Println("ANU error:", err)
	}

	//если дошли до этой части, значит с квантовыми байтами не вышло, и делаем fallback в crypto/rand
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	fmt.Printf("DEBUG: fallback на обычный генератор, рандомное число %d ", b[0])
	return b[0], nil
}

// QuantumReader реализует интерфейс io.Reader, используя квантовый источник, если всё сработало, то тут квантовые байты
type QuantumReader struct{}
func (q *QuantumReader) Read(p []byte) (n int, err error) {
	for i := range p {
		b, err := getByteFromAnySource()
		if err != nil {
			return i, err
		}
		p[i] = b
	}
	return len(p), nil
}


// функция с логикой для выбора одного варианта на основе рандомных байт
func chooseIndex(n int) (int, error) {
	if n <= 0 { //проверяем, что список не пустой
		return 0, errors.New("n must be > 0")
	}

	//Используем стандартную библиотеку crypto/rand, но в качестве источника случайности подсовываем ей QuantumReader
	idx, err := rand.Int(&QuantumReader{}, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}

	return int(idx.Int64()), nil
}

// функция API для получения рандомного значения из слайса
func GetQuantumChoice(s []string) (string, error) {
	idx, err := chooseIndex(len(s))
	if err != nil {
		return "", err
	}
	return s[idx], nil
}
