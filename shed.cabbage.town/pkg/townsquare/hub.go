package townsquare

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		switch origin {
		case "http://localhost:4321",
			"http://localhost:8080",
			"https://cabbage.town",
			"https://www.cabbage.town":
			return true
		}
		return false
	},
}

// Name generation word lists
var adjectives = []string{
	"sleepy", "bouncy", "fuzzy", "grumpy", "sneaky",
	"wobbly", "crunchy", "spicy", "toasty", "dizzy",
	"frosty", "jolly", "mellow", "perky", "zesty",
	"breezy", "chunky", "peppy", "silky", "wiggly",
}

var nouns = []string{
	"cabbage", "sprout", "turnip", "radish", "kale",
	"chard", "endive", "arugula", "kohlrabi", "fennel",
	"parsnip", "rutabaga", "celery", "broccoli", "bokchoy",
	"collard", "radicchio", "shallot", "scallion", "daikon",
}


// Message types
type msgType struct {
	Type string `json:"type"`
}

type welcomeMsg struct {
	Type  string     `json:"type"`
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	X     float64    `json:"x"`
	Y     float64    `json:"y"`
	Users []userInfo `json:"users"`
}

type userInfo struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

type joinMsg struct {
	Type string  `json:"type"`
	ID   string  `json:"id"`
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

type movedMsg struct {
	Type string  `json:"type"`
	ID   string  `json:"id"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

type leaveMsg struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type clientMoveMsg struct {
	Type string  `json:"type"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

type clientChatMsg struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type chattedMsg struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Text string `json:"text"`
}

// identity stores persistent state for a token across reconnections.
type identity struct {
	name string
	x    float64
	y    float64
}

// Client represents a single WebSocket connection.
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	id       string
	token    string
	name     string
	x        float64
	y        float64
	lastMove time.Time
	lastChat time.Time
}

// Hub manages all connected clients and broadcasts.
type Hub struct {
	mu         sync.RWMutex
	clients    map[string]*Client
	identities map[string]*identity // token → persistent identity
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		identities: make(map[string]*identity),
	}
}

func generateName() string {
	adj := adjectives[rand.Intn(len(adjectives))]
	noun := nouns[rand.Intn(len(nouns))]
	return fmt.Sprintf("%s %s", adj, noun)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// broadcast sends data to all clients except the one with excludeID.
func (h *Hub) broadcast(data []byte, excludeID string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for id, c := range h.clients {
		if id == excludeID {
			continue
		}
		select {
		case c.send <- data:
		default:
			// Drop message if buffer full
		}
	}
}

// broadcastAll sends data to all clients.
func (h *Hub) broadcastAll(data []byte) {
	h.broadcast(data, "")
}

func (h *Hub) addClient(c *Client) {
	h.mu.Lock()
	h.clients[c.id] = c
	h.mu.Unlock()
}

func (h *Hub) removeClient(id string) {
	h.mu.Lock()
	if c, ok := h.clients[id]; ok {
		close(c.send)
		delete(h.clients, id)
	}
	h.mu.Unlock()
}

func (h *Hub) getUsers(excludeID string) []userInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	users := make([]userInfo, 0, len(h.clients))
	for id, c := range h.clients {
		if id == excludeID {
			continue
		}
		users = append(users, userInfo{
			ID:   c.id,
			Name: c.name,
			X:    c.x,
			Y:    c.y,
		})
	}
	return users
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 45 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 512
	sendBufSize    = 32
	moveThrottle   = 100 * time.Millisecond
	chatThrottle   = 1 * time.Second
	maxChatLen     = 140
)

func (c *Client) readPump() {
	defer func() {
		// Save final position for reconnection
		if c.token != "" {
			c.hub.mu.Lock()
			if ident, ok := c.hub.identities[c.token]; ok {
				ident.x = c.x
				ident.y = c.y
			}
			c.hub.mu.Unlock()
		}

		c.hub.removeClient(c.id)
		c.conn.Close()

		data, _ := json.Marshal(leaveMsg{Type: "leave", ID: c.id})
		c.hub.broadcastAll(data)
		log.Printf("[townsquare] %s (%s) left", c.name, c.id)
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var mt msgType
		if err := json.Unmarshal(message, &mt); err != nil {
			continue
		}

		switch mt.Type {
		case "move":
			now := time.Now()
			if now.Sub(c.lastMove) < moveThrottle {
				continue
			}
			c.lastMove = now

			var mv clientMoveMsg
			if err := json.Unmarshal(message, &mv); err != nil {
				continue
			}

			c.x = clamp01(mv.X)
			c.y = clamp01(mv.Y)

			data, _ := json.Marshal(movedMsg{
				Type: "moved",
				ID:   c.id,
				X:    c.x,
				Y:    c.y,
			})
			c.hub.broadcast(data, c.id)

		case "chat":
			now := time.Now()
			if now.Sub(c.lastChat) < chatThrottle {
				continue
			}

			var cm clientChatMsg
			if err := json.Unmarshal(message, &cm); err != nil {
				continue
			}

			text := strings.TrimSpace(cm.Text)
			if text == "" {
				continue
			}
			if len(text) > maxChatLen {
				text = text[:maxChatLen]
			}

			c.lastChat = now

			data, _ := json.Marshal(chattedMsg{
				Type: "chatted",
				ID:   c.id,
				Text: text,
			})
			c.hub.broadcastAll(data)
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ServeWS handles WebSocket upgrade requests.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[townsquare] upgrade error: %v", err)
		return
	}

	// Parse persistent identity token
	token := r.URL.Query().Get("token")
	if len(token) > 64 {
		token = ""
	}

	var name string
	var startX, startY float64

	h.mu.Lock()
	if token != "" {
		if ident, ok := h.identities[token]; ok {
			name = ident.name
			startX = ident.x
			startY = ident.y
		} else {
			name = generateName()
			startX = 0.3 + rand.Float64()*0.4
			startY = 0.5 + rand.Float64()*0.3
			h.identities[token] = &identity{name: name, x: startX, y: startY}
		}
	} else {
		name = generateName()
		startX = 0.3 + rand.Float64()*0.4
		startY = 0.5 + rand.Float64()*0.3
	}
	h.mu.Unlock()

	client := &Client{
		hub:   h,
		conn:  conn,
		send:  make(chan []byte, sendBufSize),
		id:    fmt.Sprintf("u%d", time.Now().UnixNano()),
		token: token,
		name:  name,
		x:     startX,
		y:     startY,
	}

	// Start writePump before adding to hub so it's ready to drain send channel
	go client.writePump()

	h.addClient(client)

	// Send welcome to the new client
	welcome, _ := json.Marshal(welcomeMsg{
		Type:  "welcome",
		ID:    client.id,
		Name:  client.name,
		X:     client.x,
		Y:     client.y,
		Users: h.getUsers(client.id),
	})
	client.send <- welcome

	// Broadcast join to others
	join, _ := json.Marshal(joinMsg{
		Type: "join",
		ID:   client.id,
		Name: client.name,
		X:    client.x,
		Y:    client.y,
	})
	h.broadcast(join, client.id)

	log.Printf("[townsquare] %s (%s) joined at (%.2f, %.2f)", client.name, client.id, client.x, client.y)

	go client.readPump()
}
