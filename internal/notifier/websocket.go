package notifier

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 生产环境应限制 Origin
	},
}

type WSNotifier struct {
	clients   map[*websocket.Conn]bool
	broadcast chan []byte
	register  chan *websocket.Conn
	unregister chan *websocket.Conn
	mu        sync.RWMutex
}

func NewWSNotifier() *WSNotifier {
	wn := &WSNotifier{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
	go wn.run()
	return wn
}

func (wn *WSNotifier) Name() string { return "Websocket" }

// Send 实现 core.Notifier 接口
func (wn *WSNotifier) Send(ctx context.Context, title, message string) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"title":     title,
		"message":   message,
		"type":      "notification",
		"timestamp": context.WithValue(ctx, "ts", "now"), // 示例
	})
	
	select {
	case wn.broadcast <- payload:
	default:
		log.Println("⚠️  Websocket 广播队列已满")
	}
	return nil
}

// run 核心循环，处理连接注册与广播
func (wn *WSNotifier) run() {
	for {
		select {
		case conn := <-wn.register:
			wn.mu.Lock()
			wn.clients[conn] = true
			wn.mu.Unlock()
			log.Printf("🔌 [Websocket] 新客户端接入, 当前连接数: %d", len(wn.clients))
			
		case conn := <-wn.unregister:
			wn.mu.Lock()
			if _, ok := wn.clients[conn]; ok {
				delete(wn.clients, conn)
				conn.Close()
			}
			wn.mu.Unlock()
			
		case message := <-wn.broadcast:
			wn.mu.RLock()
			for conn := range wn.clients {
				// 每一个客户端异步发送，防止单个慢连接阻塞整个广播
				go func(c *websocket.Conn, m []byte) {
					err := c.WriteMessage(websocket.TextMessage, m)
					if err != nil {
						log.Printf("⚠️ [Websocket] 发送失败: %v", err)
						wn.unregister <- c
					}
				}(conn, message)
			}
			wn.mu.RUnlock()
		}
	}
}

// HandleUpgrade 处理 HTTP 升级到 Websocket
func (wn *WSNotifier) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ [Websocket] 升级失败: %v", err)
		return
	}
	wn.register <- conn
}
