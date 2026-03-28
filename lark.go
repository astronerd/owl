package main

import (
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// --- lark-cli wrapper ---

func runLarkCLI(args ...string) (map[string]interface{}, error) {
	cmd := exec.Command("lark-cli", args...)
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

type Chat struct {
	ID       string
	Name     string
	Mode     string // "group" or "p2p"
	LastMsg  string
	LastTime string
	UserID   string // for p2p search results
}

type Message struct {
	ID         string
	Sender     string
	SenderID   string
	SenderType string
	Content    string
	MsgType    string
	Time       string
}

func getMyOpenID() string {
	r, err := runLarkCLI("api", "GET", "/open-apis/authen/v1/user_info", "--as", "user")
	if err != nil {
		return ""
	}
	if data, ok := r["data"].(map[string]interface{}); ok {
		if id, ok := data["open_id"].(string); ok {
			return id
		}
	}
	return ""
}

func getChatList(limit int) []Chat {
	params, _ := json.Marshal(map[string]string{
		"page_size": "50",
		"sort_type": "ByActiveTimeDesc",
	})
	r, err := runLarkCLI("im", "chats", "list", "--as", "user",
		"--params", string(params), "--format", "json")
	if err != nil {
		return nil
	}
	data, _ := r["data"].(map[string]interface{})
	items, _ := data["items"].([]interface{})

	var chats []Chat
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		chats = append(chats, Chat{
			ID:   str(m, "chat_id"),
			Name: str(m, "name"),
			Mode: "group",
		})
		if len(chats) >= limit {
			break
		}
	}
	return chats
}

func getP2PChats(myOpenID string, limit int) []Chat {
	r, err := runLarkCLI("im", "+messages-search", "--as", "user",
		"--chat-type", "p2p", "--page-size", "50", "--format", "json")
	if err != nil {
		return nil
	}
	data, _ := r["data"].(map[string]interface{})
	msgs, _ := data["messages"].([]interface{})

	seen := map[string]bool{}
	var chats []Chat
	for _, msg := range msgs {
		m, _ := msg.(map[string]interface{})
		chatID := str(m, "chat_id")
		if chatID == "" || seen[chatID] {
			continue
		}
		seen[chatID] = true

		sender, _ := m["sender"].(map[string]interface{})
		senderID := str(sender, "id")
		senderName := str(sender, "name")

		name := ""
		if senderID != myOpenID && senderName != "" {
			name = senderName
		}
		if name == "" {
			// look at more messages
			chatMsgs := getMessages(chatID, 5)
			for _, cm := range chatMsgs {
				if cm.SenderID != myOpenID && cm.SenderType == "user" && cm.Sender != "" {
					name = cm.Sender
					break
				}
			}
		}
		if name == "" || strings.HasPrefix(name, "cli_") {
			continue
		}

		lastMsg := str(m, "content")
		if len(lastMsg) > 30 {
			lastMsg = lastMsg[:30]
		}

		chats = append(chats, Chat{
			ID:       chatID,
			Name:     name,
			Mode:     "p2p",
			LastMsg:  lastMsg,
			LastTime: str(m, "create_time"),
		})
		if len(chats) >= limit {
			break
		}
	}
	return chats
}

func getMessages(chatID string, limit int) []Message {
	r, err := runLarkCLI("im", "+chat-messages-list", "--as", "user",
		"--chat-id", chatID, "--page-size", string(rune('0'+limit)),
		"--sort", "desc", "--format", "json")
	if err != nil {
		return nil
	}
	if ok, _ := r["ok"].(bool); !ok {
		return nil
	}
	data, _ := r["data"].(map[string]interface{})
	items, _ := data["messages"].([]interface{})

	var msgs []Message
	for i := len(items) - 1; i >= 0; i-- {
		m, _ := items[i].(map[string]interface{})
		sender, _ := m["sender"].(map[string]interface{})
		msgs = append(msgs, Message{
			ID:         str(m, "message_id"),
			Sender:     str(sender, "name"),
			SenderID:   str(sender, "id"),
			SenderType: str(sender, "sender_type"),
			Content:    str(m, "content"),
			MsgType:    str(m, "msg_type"),
			Time:       str(m, "create_time"),
		})
	}
	return msgs
}

func getMessagesStr(chatID string, limit int) []Message {
	limitStr := "30"
	if limit < 10 {
		limitStr = string(rune('0' + limit))
	} else if limit <= 50 {
		limitStr = strings.TrimLeft(json.Number(json.Number(string(rune('0'+limit/10))).String()+json.Number(string(rune('0'+limit%10))).String()).String(), "0")
	}
	_ = limitStr
	// Just use the proper way
	return getMessagesPaged(chatID, limit)
}

func getMessagesPaged(chatID string, limit int) []Message {
	r, err := runLarkCLI("im", "+chat-messages-list", "--as", "user",
		"--chat-id", chatID, "--page-size", itoa(limit),
		"--sort", "desc", "--format", "json")
	if err != nil {
		return nil
	}
	if ok, _ := r["ok"].(bool); !ok {
		return nil
	}
	data, _ := r["data"].(map[string]interface{})
	items, _ := data["messages"].([]interface{})

	var msgs []Message
	for i := len(items) - 1; i >= 0; i-- {
		m, _ := items[i].(map[string]interface{})
		sender, _ := m["sender"].(map[string]interface{})
		msgs = append(msgs, Message{
			ID:         str(m, "message_id"),
			Sender:     str(sender, "name"),
			SenderID:   str(sender, "id"),
			SenderType: str(sender, "sender_type"),
			Content:    str(m, "content"),
			MsgType:    str(m, "msg_type"),
			Time:       str(m, "create_time"),
		})
	}
	return msgs
}

func sendMessage(chatID, text string) error {
	// Use Python helper for sending (user token managed there)
	cmd := exec.Command("python3", "-c", `
import sys, os
sys.path.insert(0, os.path.expanduser("~/feishu-tui"))
os.environ["FEISHU_APP_ID"] = "`+appID+`"
os.environ["FEISHU_APP_SECRET"] = "`+appSecret+`"
from feishu_api import send_message
r = send_message("`+chatID+`", """`+strings.ReplaceAll(text, `"`, `\"`)+`""")
if r.get("code") != 0:
    print(r.get("msg","error"), file=sys.stderr)
    sys.exit(1)
`)
	return cmd.Run()
}

func searchUsers(query string) []Chat {
	r, err := runLarkCLI("contact", "+search-user", "--as", "user",
		"--query", query, "--page-size", "5", "--format", "json")
	if err != nil {
		return nil
	}
	if ok, _ := r["ok"].(bool); !ok {
		return nil
	}
	data, _ := r["data"].(map[string]interface{})
	users, _ := data["users"].([]interface{})

	var chats []Chat
	for _, u := range users {
		m, _ := u.(map[string]interface{})
		chats = append(chats, Chat{
			Name:   str(m, "name"),
			Mode:   "p2p",
			UserID: str(m, "open_id"),
		})
	}
	return chats
}

// helpers

func str(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func formatTime(t string) string {
	// Input: "2026-03-28 21:57" or similar
	parsed, err := time.Parse("2006-01-02 15:04", t)
	if err != nil {
		// Try other format
		parsed, err = time.Parse("2006-01-02 15:04:05", t)
		if err != nil {
			return t
		}
	}
	now := time.Now()
	if parsed.Year() == now.Year() && parsed.YearDay() == now.YearDay() {
		return parsed.Format("15:04")
	}
	return parsed.Format("01-02")
}

var (
	appID     = ""
	appSecret = ""
)
