package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite (Tanpa CGO)
)

var db *sql.DB

// Konstanta Rahasia untuk Enkripsi Hardware ID (OpenClaw Project Guardrails)
const SecretSalt = "OPENCLAW_AI_SECRET_2026_PRO"

// Struktur data untuk OpenAI-Compatible Payload
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func main() {
	// ==========================================================
	// 1. MODE ADMIN (Generator Kunci Lisensi)
	// ==========================================================
	if len(os.Args) > 1 && os.Args[1] == "-admin" {
		fmt.Println("======================================")
		fmt.Println("   MODE GENERATOR LISENSI (ADMIN)     ")
		fmt.Println("======================================")
		fmt.Println("Volume ID Fisik :", getVolumeID())
		fmt.Println("License Key     :", generateExpectedKey())
		fmt.Println("======================================")
		return
	}

	fmt.Println("=== PORTABLE AI STUDIO ===")
	fmt.Println("Memeriksa keamanan perangkat keras...")

	// ==========================================================
	// 2. SISTEM OTENTIKASI & HARDWARE LOCK
	// ==========================================================
	expectedKey := generateExpectedKey()
	savedKey, err := os.ReadFile("license.key")
	
	if err == nil && strings.TrimSpace(string(savedKey)) == expectedKey {
		fmt.Println("[OK] Lisensi tervalidasi untuk perangkat ini.")
	} else {
		fmt.Print("\n[PERHATIAN] Aplikasi belum diaktivasi untuk USB ini.\nMasukkan License Key Anda: ")
		reader := bufio.NewReader(os.Stdin)
		inputKey, _ := reader.ReadString('\n')
		inputKey = strings.TrimSpace(inputKey)
		
		if inputKey == expectedKey {
			os.WriteFile("license.key", []byte(expectedKey), 0644)
			fmt.Println("\n[BERHASIL] Aktivasi selesai! Lisensi telah disimpan.")
		} else {
			fmt.Println("\n[GAGAL] License Key salah atau perangkat tidak dikenali.")
			fmt.Println("Menutup program dalam 3 detik...")
			time.Sleep(3 * time.Second)
			os.Exit(1)
		}
	}

	// ==========================================================
	// 3. INISIALISASI SISTEM & SERVER
	// ==========================================================
	fmt.Println("\nMenghidupkan AI Engine lokal...")
	initDB()
	go startEngine()

	// Routing Web Server
	http.Handle("/", http.FileServer(http.Dir("./public")))
	http.HandleFunc("/api/chat", chatHandler)

	fmt.Println("🚀 Sistem siap! Berjalan di http://localhost:8080")

	// Buka browser otomatis setelah engine punya waktu untuk menyala
	go func() {
		time.Sleep(3 * time.Second)
		openBrowser("http://localhost:8080")
	}()

	log.Fatal(http.ListenAndServe(":8080", nil))
}

// ==========================================================
// FUNGSI MANAJEMEN DATABASE
// ==========================================================
func initDB() {
	var err error
	db, err = sql.Open("sqlite", "./data/memory.db")
	if err != nil {
		log.Fatal("Gagal memuat memori lokal:", err)
	}
	// Buat tabel history jika belum ada
	createTableQuery := `CREATE TABLE IF NOT EXISTS chat_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT, 
		role TEXT, 
		content TEXT
	)`
	db.Exec(createTableQuery)
}

// ==========================================================
// FUNGSI KONTROL AI ENGINE
// ==========================================================
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

	// Menggunakan context window 4096 (Standar Llama 3)
	cmd := exec.Command(enginePath, "-m", "./models/model.gguf", "--port", "8081", "-c", "4096")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// ==========================================================
// FUNGSI API (MIDDLEWARE & STREAMING)
// ==========================================================
func chatHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Tarik 10 memori terakhir untuk memberikan konteks jangka pendek
	rows, _ := db.Query("SELECT role, content FROM (SELECT role, content FROM chat_history ORDER BY id DESC LIMIT 10) ORDER BY id ASC")
	var messages []Message
	
	// System Guardrails
	messages = append(messages, Message{
		Role:    "system", 
		Content: "Anda adalah asisten AI offline yang sangat cerdas. Selalu gunakan format Markdown untuk menjawab.",
	})
	
	for rows.Next() {
		var role, content string
		rows.Scan(&role, &content)
		messages = append(messages, Message{Role: role, Content: content})
	}
	rows.Close()

	// Tambahkan pesan user terbaru
	messages = append(messages, Message{Role: "user", Content: req.Prompt})

	// Setup Headers untuk Streaming (SSE)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	payload, _ := json.Marshal(map[string]interface{}{
		"messages": messages,
		"stream":   true,
	})

	// Teruskan request ke llama-server lokal
	resp, err := http.Post("http://127.0.0.1:8081/v1/chat/completions", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		http.Error(w, "AI Engine belum siap", 500)
		return
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	flusher, _ := w.(http.Flusher)
	var fullResponse strings.Builder

	// Parsing stream SSE
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

	// Tulis log memori kembali ke database (Local Memory Write-back)
	db.Exec("INSERT INTO chat_history (role, content) VALUES (?, ?)", "user", req.Prompt)
	db.Exec("INSERT INTO chat_history (role, content) VALUES (?, ?)", "assistant", fullResponse.String())
}

// ==========================================================
// FUNGSI UTILITAS OS
// ==========================================================
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

func generateExpectedKey() string {
	volID := getVolumeID()
	if volID == "UNKNOWN" {
		volID = "DEFAULT_SECURE_DRIVE_2026"
	}
	hashBytes := sha256.Sum256([]byte(volID + SecretSalt))
	fullHash := hex.EncodeToString(hashBytes[:])
	return strings.ToUpper(fullHash[:16]) // Kembalikan 16 karakter kunci
}

func getVolumeID() string {
	dir, _ := os.Getwd()
	switch runtime.GOOS {
	case "windows":
		drive := filepath.VolumeName(dir)
		out, err := exec.Command("cmd", "/c", "vol", drive).Output()
		if err == nil {
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				if strings.Contains(strings.ToLower(line), "serial number") {
					parts := strings.Split(line, ":")
					if len(parts) > 1 {
						return strings.TrimSpace(parts[1])
					}
				}
			}
		}
	case "darwin":
		out, err := exec.Command("sh", "-c", "df -m . | tail -1 | awk '{print $1}' | xargs diskutil info | grep 'Volume UUID' | awk '{print $3}'").Output()
		if err == nil && len(out) > 0 {
			return strings.TrimSpace(string(out))
		}
	case "linux":
		out, err := exec.Command("sh", "-c", "df . | tail -1 | awk '{print $1}' | xargs blkid -s UUID -o value").Output()
		if err == nil && len(out) > 0 {
			return strings.TrimSpace(string(out))
		}
	}
	return "UNKNOWN"
}