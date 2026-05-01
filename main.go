package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

// Struktur untuk payload OpenAI API Format
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func main() {
	initDB()
	go startEngine()

	http.Handle("/", http.FileServer(http.Dir("./public")))
	http.HandleFunc("/api/chat", chatHandler)

	fmt.Println("🚀 AI Middleware berjalan di http://localhost:8080")
	
	// Auto-open browser setelah 3 detik
	go func() {
		time.Sleep(3 * time.Second)
		openBrowser("http://localhost:8080")
	}()

	log.Fatal(http.ListenAndServe(":8080", nil))
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite", "./data/memory.db")
	if err != nil {
		log.Fatal("Gagal memuat memori:", err)
	}
	db.Exec("CREATE TABLE IF NOT EXISTS chat_history (id INTEGER PRIMARY KEY AUTOINCREMENT, role TEXT, content TEXT)")
}

func startEngine() {
	var enginePath string
	switch runtime.GOOS {
	case "windows":
		enginePath = "./bin/llama-server-win.exe"
	case "darwin":
		enginePath = "./bin/llama-server-mac"
	case "linux":
		enginePath = "./bin/llama-server-linux"
	default:
		log.Fatal("OS tidak didukung")
	}

	cmd := exec.Command(enginePath, "-m", "./models/model.gguf", "--port", "8081", "-c", "4096")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
	case "linux":
		exec.Command("xdg-open", url).Start()
	}
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// 1. Ambil 10 memori terakhir dari SQLite untuk konteks (Long-term memory dasar)
	rows, _ := db.Query("SELECT role, content FROM (SELECT role, content FROM chat_history ORDER BY id DESC LIMIT 10) ORDER BY id ASC")
	var messages []Message
	
	// System prompt (Bisa disesuaikan)
	messages = append(messages, Message{Role: "system", Content: "Anda adalah asisten AI offline yang berjalan dari USB. Jawab dengan markdown yang rapi."})
	
	for rows.Next() {
		var role, content string
		rows.Scan(&role, &content)
		messages = append(messages, Message{Role: role, Content: content})
	}
	rows.Close()

	// Masukkan prompt terbaru
	messages = append(messages, Message{Role: "user", Content: req.Prompt})

	// 2. Setup Headers untuk Streaming ke Frontend
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Transfer-Encoding", "chunked")

	// 3. Payload untuk llama.cpp (/v1/chat/completions)
	payload, _ := json.Marshal(map[string]interface{}{
		"messages": messages,
		"stream":   true,
	})

	resp, err := http.Post("http://127.0.0.1:8081/v1/chat/completions", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		http.Error(w, "Engine belum siap", 500)
		return
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	flusher, _ := w.(http.Flusher)
	var fullResponse strings.Builder

	// 4. Proses Stream dari Engine AI
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		
		if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimSpace(line[6:])
			if dataStr == "[DONE]" {
				break
			}

			var data map[string]interface{}
			json.Unmarshal([]byte(dataStr), &data)
			
			// Parsing format chunk dari OpenAI-compatible API
			if choices, ok := data["choices"].([]interface{}); ok && len(choices) > 0 {
				if delta, ok := choices[0].(map[string]interface{})["delta"].(map[string]interface{}); ok {
					if content, ok := delta["content"].(string); ok {
						fmt.Fprint(w, content)
						fullResponse.WriteString(content)
						flusher.Flush()
					}
				}
			}
		}
	}

	// 5. Simpan ke Database
	db.Exec("INSERT INTO chat_history (role, content) VALUES (?, ?)", "user", req.Prompt)
	db.Exec("INSERT INTO chat_history (role, content) VALUES (?, ?)", "assistant", fullResponse.String())
}