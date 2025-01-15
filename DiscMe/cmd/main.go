package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Cliente representa uma conexão WebSocket
type Client struct {
	conn *websocket.Conn
	send chan []byte
}

// ChatServer gerencia as conexões e mensagens do chat
type ChatServer struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mutex      sync.Mutex
}

// Configuração do upgrader WebSocket
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Permitir qualquer origem para desenvolvimento
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Criar novo servidor de chat
func NewChatServer() *ChatServer {
	return &ChatServer{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Iniciar o servidor de chat
func (cs *ChatServer) Run() {
	for {
		select {
		case client := <-cs.register:
			cs.mutex.Lock()
			cs.clients[client] = true
			cs.mutex.Unlock()
			log.Printf("Novo cliente conectado. Total: %d", len(cs.clients))

		case client := <-cs.unregister:
			cs.mutex.Lock()
			if _, ok := cs.clients[client]; ok {
				delete(cs.clients, client)
				close(client.send)
			}
			cs.mutex.Unlock()
			log.Printf("Cliente desconectado. Total: %d", len(cs.clients))

		case message := <-cs.broadcast:
			cs.mutex.Lock()
			for client := range cs.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(cs.clients, client)
				}
			}
			cs.mutex.Unlock()
		}
	}
}

// Gerenciar conexão WebSocket para cada cliente
func (cs *ChatServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Erro ao fazer upgrade da conexão:", err)
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
	}
	cs.register <- client

	// Goroutines para leitura e escrita
	go client.writePump()
	go client.readPump(cs)
}

// Ler mensagens do cliente
func (c *Client) readPump(cs *ChatServer) {
	defer func() {
		cs.unregister <- c
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Erro: %v", err)
			}
			break
		}
		cs.broadcast <- message
	}
}

// Enviar mensagens para o cliente
func (c *Client) writePump() {
	defer func() {
		c.conn.Close()
	}()

	for {
		message, ok := <-c.send
		if !ok {
			c.conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}

		err := c.conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			log.Printf("Erro ao enviar mensagem: %v", err)
			return
		}
	}
}

func main() {
	chatServer := NewChatServer()
	go chatServer.Run()

	// Rota para WebSocket
	http.HandleFunc("/ws", chatServer.handleWebSocket)

	// Rota básica para verificar se o servidor está rodando
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Servidor WebSocket Iniciado !!!")
	})

	port := ":8080"
	log.Printf("Servidor iniciado na porta %s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
