package maxclientapi

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ChatClient представляет клиент для работы с чатом
type ChatClient struct {
	URL            string
	Token          string
	WatchChats     []string
	UserAgent      string
	DeviceName     string
	HeaderUserAgent string
	OSVersion      string
	DeviceID       string
	Origin         string
	
	ws             *websocket.Conn
	seq            int
	running        bool
	messages       chan map[string]interface{}
	allowReconnect bool
	debug          bool
	mu             sync.Mutex
	stopChan       chan struct{}
}

// NewChatClient создает новый экземпляр клиента
func NewChatClient(token, deviceID string, options ...Option) *ChatClient {
	client := &ChatClient{
		URL:             "wss://ws-api.oneme.ru/websocket",
		Token:           token,
		DeviceID:        deviceID,
		WatchChats:      []string{},
		UserAgent:       "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:144.0) Gecko/20100101 Firefox/144.0",
		DeviceName:      "Firefox",
		OSVersion:       "Linux",
		Origin:          "https://web.max.ru",
		messages:        make(chan map[string]interface{}, 100),
		allowReconnect:  false,
		debug:           false,
		seq:             0,
		running:         false,
		stopChan:        make(chan struct{}),
	}

	for _, option := range options {
		option(client)
	}

	client.HeaderUserAgent = client.UserAgent
	return client
}

// Option определяет функциональные опции для клиента
type Option func(*ChatClient)

func WithWatchChats(chats []string) Option {
	return func(c *ChatClient) {
		c.WatchChats = chats
	}
}

func WithUserAgent(userAgent string) Option {
	return func(c *ChatClient) {
		c.UserAgent = userAgent
	}
}

func WithDebug(debug bool) Option {
	return func(c *ChatClient) {
		c.debug = debug
	}
}

func WithAllowReconnect(allow bool) Option {
	return func(c *ChatClient) {
		c.allowReconnect = allow
	}
}

// Connect устанавливает WebSocket соединение
func (c *ChatClient) Connect() error {
	if c.allowReconnect {
		log.Println("[MAXCLIENTAPI] auto reconnect turned on")
	}
	if c.debug {
		log.Println("[MAXCLIENTAPI] debug mode is turned on")
	}

	headers := map[string][]string{
		"Origin":     {c.Origin},
		"User-Agent": {c.UserAgent},
	}

	log.Println("[MAXCLIENTAPI] Trying to connect")
	
	dialer := websocket.Dialer{}
	ws, _, err := dialer.Dial(c.URL, headers)
	if err != nil {
		return fmt.Errorf("connection error: %w", err)
	}

	c.ws = ws
	c.running = true
	log.Println("[MAXCLIENTAPI] WebSocket connected")

	go c.listenHandler()
	c.sendInfo()

	return nil
}

// sendInfo отправляет информацию об устройстве
func (c *ChatClient) sendInfo() {
	c.seq++
	payload := map[string]interface{}{
		"ver":    11,
		"cmd":    0,
		"seq":    c.seq,
		"opcode": 6,
		"payload": map[string]interface{}{
			"userAgent": map[string]interface{}{
				"deviceType":       "WEB",
				"locale":           "ru",
				"deviceLocale":     "ru",
				"osVersion":        c.OSVersion,
				"deviceName":       c.DeviceName,
				"headerUserAgent":  c.HeaderUserAgent,
				"appVersion":       "25.11.1",
				"screen":           "1080x1920 1.0x",
				"timezone":         "Asia/Yekaterinburg",
			},
			"deviceId": c.DeviceID,
		},
	}
	c.send(payload, "Info")
	c.sendHandshake()
}

// sendHandshake отправляет handshake
func (c *ChatClient) sendHandshake() {
	log.Println("Sending handshake")
	c.seq++
	payload := map[string]interface{}{
		"ver":    11,
		"cmd":    0,
		"seq":    c.seq,
		"opcode": 19,
		"payload": map[string]interface{}{
			"interactive":   true,
			"token":         c.Token,
			"chatsCount":    len(c.WatchChats),
			"chatsSync":     0,
			"contactsSync":  0,
			"presenceSync":  0,
			"draftsSync":    0,
		},
	}
	c.send(payload, "Handshake")
}

// listenHandler обрабатывает входящие сообщения
func (c *ChatClient) listenHandler() {
	defer func() {
		c.Stop()
		log.Println("[MAXCLIENTAPI][LISTEN_HANDLER] WebSocket stop")
	}()

	for c.running {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				log.Println("Connection closed")
			} else {
				log.Printf("Read error: %v", err)
			}
			break
		}

		var jsonData map[string]interface{}
		if err := json.Unmarshal(message, &jsonData); err != nil {
			log.Printf("JSON parse error: %v", err)
			continue
		}

		c.handleMessage(jsonData)
	}
}

// handleMessage обрабатывает полученное сообщение
func (c *ChatClient) handleMessage(jsonData map[string]interface{}) {
	opcode, ok := jsonData["opcode"].(float64)
	if !ok {
		return
	}

	switch int(opcode) {
	case 128:
		c.handleOpcode128(jsonData)
	case 83:
		c.handleOpcode83(jsonData)
	case 87:
		c.handleOpcode87(jsonData)
	}
}

// handleOpcode128 обрабатывает сообщения с opcode 128
func (c *ChatClient) handleOpcode128(jsonData map[string]interface{}) {
	payload, _ := jsonData["payload"].(map[string]interface{})
	messageData, _ := payload["message"].(map[string]interface{})
	chatID := payload["chatId"]
	sender := messageData["sender"]
	text, _ := messageData["text"].(string)
	attaches, _ := messageData["attaches"].([]interface{})
	elements, _ := messageData["elements"].([]interface{})

	if len(attaches) > 0 {
		for _, attach := range attaches {
			attachMap, _ := attach.(map[string]interface{})
			mediaType, _ := attachMap["_type"].(string)

			switch mediaType {
			case "PHOTO":
				c.handlePhotoAttach(attachMap, chatID, sender, text, messageData, payload)
			case "VIDEO":
				c.handleVideoAttach(attachMap, chatID, sender, text, messageData, payload)
			case "FILE":
				c.handleFileAttach(attachMap, chatID, sender, messageData)
			case "SHARE":
				c.handleShareAttach(chatID, sender, text)
			}
		}
	} else if len(elements) > 0 {
		// Обработка elements (если есть SHARE)
		c.handleShareAttach(chatID, sender, text)
	} else if text != "" {
		textInfo := map[string]interface{}{
			"opcode":        128,
			"type":          "text",
			"chat_id":       chatID,
			"sender":        sender,
			"text":          text,
			"id":            messageData["id"],
			"time":          messageData["time"],
			"utype":         messageData["type"],
			"prevMessageId": payload["prevMessageId"],
		}
		c.messages <- textInfo
		fmt.Printf("Text from %v: %s\n", sender, text)
	}
}

// handlePhotoAttach обрабатывает фото вложения
func (c *ChatClient) handlePhotoAttach(attach map[string]interface{}, chatID, sender interface{}, text string, messageData, payload map[string]interface{}) {
	mediaType := "photo"
	if text != "" {
		mediaType = "photo_with_text"
	}

	mediaInfo := map[string]interface{}{
		"opcode":      128,
		"type":        mediaType,
		"chat_id":     chatID,
		"sender":      sender,
		"id":          messageData["id"],
		"time":        messageData["time"],
		"utype":       messageData["type"],
		"baseUrl":     attach["baseUrl"],
		"previewData": attach["previewData"],
		"photoToken":  attach["photoToken"],
		"width":       attach["width"],
		"photoId":     attach["photoId"],
		"height":      attach["height"],
		"raw":         attach,
	}

	if text != "" {
		mediaInfo["text"] = text
		fmt.Printf("Photo with text from %v\n", sender)
	} else {
		fmt.Printf("Photo from %v\n", sender)
	}

	c.messages <- mediaInfo
}

// handleVideoAttach обрабатывает видео вложения
func (c *ChatClient) handleVideoAttach(attach map[string]interface{}, chatID, sender interface{}, text string, messageData, payload map[string]interface{}) {
	mediaType := "video"
	if text != "" {
		mediaType = "video_with_text"
	}

	mediaInfo := map[string]interface{}{
		"opcode":        128,
		"type":          mediaType,
		"chat_id":       chatID,
		"sender":        sender,
		"thumbnail":     attach["thumbnail"],
		"duration":      attach["duration"],
		"width":         attach["width"],
		"videoId":       attach["videoId"],
		"token":         attach["token"],
		"height":        attach["height"],
		"raw":           attach,
		"id":            messageData["id"],
		"time":          messageData["time"],
		"utype":         messageData["type"],
		"prevMessageId": payload["prevMessageId"],
	}

	if text != "" {
		mediaInfo["text"] = text
		fmt.Printf("Video with text from %v\n", sender)
	} else {
		fmt.Printf("Video from %v\n", sender)
	}

	c.messages <- mediaInfo
}

// handleFileAttach обрабатывает файловые вложения
func (c *ChatClient) handleFileAttach(attach map[string]interface{}, chatID, sender interface{}, messageData map[string]interface{}) {
	mediaInfo := map[string]interface{}{
		"opcode": 128,
		"type":   "file",
		"chatId": chatID,
		"sender": sender,
		"id":     messageData["id"],
		"name":   attach["name"],
		"size":   attach["size"],
		"fileId": attach["fileId"],
		"token":  attach["token"],
	}
	c.messages <- mediaInfo
}

// handleShareAttach обрабатывает ссылки
func (c *ChatClient) handleShareAttach(chatID, sender interface{}, text string) {
	mediaInfo := map[string]interface{}{
		"opcode":  128,
		"type":    "link",
		"chat_id": chatID,
		"sender":  sender,
		"text":    text,
	}
	c.messages <- mediaInfo
	fmt.Printf("Link from %v\n", sender)
}

// handleOpcode83 обрабатывает сообщения с opcode 83
func (c *ChatClient) handleOpcode83(jsonData map[string]interface{}) {
	payload, _ := jsonData["payload"].(map[string]interface{})
	
	var videoURL interface{}
	i := 0
	for _, v := range payload {
		if i == 2 {
			videoURL = v
			break
		}
		i++
	}

	downloadInfo := map[string]interface{}{
		"opcode":    83,
		"type":      "download_link",
		"EXTERNAL":  payload["EXTERNAL"],
		"video_url": videoURL,
		"raw":       payload,
	}
	c.messages <- downloadInfo
	log.Println("Gotted url")
}

// handleOpcode87 обрабатывает сообщения с opcode 87
func (c *ChatClient) handleOpcode87(jsonData map[string]interface{}) {
	payload, _ := jsonData["payload"].(map[string]interface{})
	info, _ := payload["info"].([]interface{})
	
	if len(info) > 0 {
		infoMap, _ := info[0].(map[string]interface{})
		urlFrom87 := map[string]interface{}{
			"opcode": 87,
			"type":   "url_upload",
			"url":    infoMap["url"],
			"token":  infoMap["token"],
			"fileId": infoMap["fileId"],
		}
		c.messages <- urlFrom87
	}
}

// StartKeepalive запускает отправку keepalive пакетов
func (c *ChatClient) StartKeepalive(interval time.Duration) {
	if interval == 0 {
		interval = 25 * time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if !c.running {
					return
				}
				c.seq++
				pingPayload := map[string]interface{}{
					"ver":    11,
					"cmd":    0,
					"seq":    c.seq,
					"opcode": 1,
					"payload": map[string]interface{}{
						"interactive": false,
					},
				}
				c.send(pingPayload, "")
			case <-c.stopChan:
				return
			}
		}
	}()
}

// GetMessage получает сообщение из очереди
func (c *ChatClient) GetMessage() (map[string]interface{}, bool) {
	select {
	case msg := <-c.messages:
		return msg, true
	default:
		return nil, false
	}
}

// GetMessageBlocking получает сообщение из очереди (блокирующий вызов)
func (c *ChatClient) GetMessageBlocking() map[string]interface{} {
	return <-c.messages
}

// SendMessage отправляет текстовое сообщение
func (c *ChatClient) SendMessage(chatID interface{}, text string) {
	c.seq++
	payload := map[string]interface{}{
		"ver":    11,
		"cmd":    0,
		"seq":    c.seq,
		"opcode": 64,
		"payload": map[string]interface{}{
			"chatId": chatID,
			"message": map[string]interface{}{
				"text":     text,
				"cid":      time.Now().UnixMilli(),
				"elements": []interface{}{},
				"attaches": []interface{}{},
			},
			"notify": true,
		},
	}
	c.send(payload, "Message")

	c.seq++
	payload1 := map[string]interface{}{
		"ver":    11,
		"cmd":    0,
		"seq":    c.seq,
		"opcode": 177,
		"payload": map[string]interface{}{
			"chatId": chatID,
			"time":   0,
		},
	}
	c.send(payload1, "")
}

// GetVideoURL запрашивает URL видео
func (c *ChatClient) GetVideoURL(videoID, chatID, messageID interface{}) {
	c.seq++
	payload := map[string]interface{}{
		"ver":    11,
		"cmd":    0,
		"seq":    c.seq,
		"opcode": 83,
		"payload": map[string]interface{}{
			"videoId":   videoID,
			"chatId":    chatID,
			"messageId": messageID,
		},
	}
	c.send(payload, "Get video URL")
}

// SubscribeChat подписывается на чат
func (c *ChatClient) SubscribeChat(chatID interface{}) {
	c.seq++
	payload := map[string]interface{}{
		"ver":    11,
		"cmd":    0,
		"seq":    c.seq,
		"opcode": 75,
		"payload": map[string]interface{}{
			"chatId":    chatID,
			"subscribe": true,
		},
	}
	c.send(payload, "Subscribe chat")
}

// RequestURLToSendFile запрашивает URL для отправки файла
func (c *ChatClient) RequestURLToSendFile(count int) {
	c.seq++
	payload := map[string]interface{}{
		"ver":    11,
		"cmd":    0,
		"seq":    c.seq,
		"opcode": 87,
		"payload": map[string]interface{}{
			"count": count,
		},
	}
	c.send(payload, "Request URL to send file")
}

// SendFile отправляет файл
func (c *ChatClient) SendFile(chatID, fileID interface{}) {
	c.seq++
	payload := map[string]interface{}{
		"ver":    11,
		"cmd":    0,
		"seq":    c.seq,
		"opcode": 64,
		"payload": map[string]interface{}{
			"chatId": chatID,
			"message": map[string]interface{}{
				"cid": time.Now().UnixMilli(),
				"attaches": []interface{}{
					map[string]interface{}{
						"_type":  "FILE",
						"fileId": fileID,
					},
				},
			},
			"notify": true,
		},
	}
	c.send(payload, "File")
}

// send отправляет данные через WebSocket
func (c *ChatClient) send(data map[string]interface{}, sendType string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ws == nil {
		log.Println("[MAXCLIENTAPI] WebSocket connection is closed")
		c.Stop()
		if c.allowReconnect {
			go func() {
				time.Sleep(2 * time.Second)
				c.Connect()
				c.StartKeepalive(25 * time.Second)
			}()
		}
		return
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("[ERROR]: %v", err)
		return
	}

	err = c.ws.WriteMessage(websocket.TextMessage, jsonData)
	if err != nil {
		log.Printf("[ERROR]: %v", err)
		return
	}

	if c.debug {
		log.Printf("[MAXCLIENTAPI] The %s successfully sent", sendType)
		log.Printf("[MAXCLIENTAPI] Data: %s", string(jsonData))
	} else if sendType != "" {
		log.Printf("[MAXCLIENTAPI] The %s successfully sent", sendType)
	}
}

// Stop останавливает клиент
func (c *ChatClient) Stop() {
	c.running = false
	close(c.stopChan)
	if c.ws != nil {
		c.ws.Close()
	}
}