package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- styles ---

var (
	blue   = lipgloss.Color("4")
	green  = lipgloss.Color("2")
	dim    = lipgloss.Color("8")
	white  = lipgloss.Color("15")
	yellow = lipgloss.Color("3")

	titleActive = lipgloss.NewStyle().Foreground(blue).Bold(true)
	titleInact  = lipgloss.NewStyle().Foreground(dim)

	nameStyle = lipgloss.NewStyle().Foreground(white).Bold(true)
	selStyle  = lipgloss.NewStyle().Reverse(true).Bold(true)
	dimStyle  = lipgloss.NewStyle().Foreground(dim)
	helpKey   = lipgloss.NewStyle().Foreground(yellow)

	greenDot = lipgloss.NewStyle().Foreground(green).Render("●")
	blueDot  = lipgloss.NewStyle().Foreground(blue).Render("●")
	greenBar = lipgloss.NewStyle().Foreground(green).Render("│")
	blueBar  = lipgloss.NewStyle().Foreground(blue).Render("│")
)

type focus int

const (
	focusChatList focus = iota
	focusMessages
	focusInput
	focusSearch
)

// --- tea messages ---

type chatsLoaded struct{ chats []Chat }
type msgsLoaded struct {
	chatID string
	msgs   []Message
}
type msgSent struct{ chatID string }
type lastMsgLoaded struct {
	chatID, lastMsg, lastTime string
}

// --- model ---

type model struct {
	w, h           int
	focus          focus
	chats, allChats []Chat
	chatIdx, chatScroll int
	msgs           []Message
	msgScroll      int
	myOpenID       string
	activeChatID   string
	activeChatName string
	input          textinput.Model
	search         textinput.Model
	spin           spinner.Model
	searching      bool
	loading        bool
	loadingMsgs    bool
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "type a message..."
	ti.CharLimit = 2000
	si := textinput.New()
	si.Placeholder = "search..."
	si.CharLimit = 64
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(blue)

	appID = os.Getenv("FEISHU_APP_ID")
	appSecret = os.Getenv("FEISHU_APP_SECRET")
	return model{input: ti, search: si, spin: sp, loading: true}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(loadChats, tea.WindowSize(), tea.EnableMouseAllMotion, m.spin.Tick)
}

func loadChats() tea.Msg {
	myID := getMyOpenID()
	return chatsLoaded{chats: append(getP2PChats(myID, 10), getChatList(30)...)}
}

func loadMsgs(id string) tea.Cmd {
	return func() tea.Msg { return msgsLoaded{id, getMessagesPaged(id, 30)} }
}

func loadLastMsg(id string) tea.Cmd {
	return func() tea.Msg {
		msgs := getMessagesPaged(id, 1)
		if len(msgs) > 0 {
			m := msgs[0]
			c := m.Content
			switch m.MsgType {
			case "image":
				c = "[image]"
			case "file":
				c = "[file]"
			}
			p := c
			if m.Sender != "" {
				p = m.Sender + ": " + c
			}
			r := []rune(p)
			if len(r) > 35 {
				p = string(r[:35])
			}
			return lastMsgLoaded{id, p, m.Time}
		}
		return lastMsgLoaded{chatID: id}
	}
}

func doSend(id, text string) tea.Cmd {
	return func() tea.Msg { sendMessage(id, text); return msgSent{id} }
}

// --- update ---

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case chatsLoaded:
		m.loading = false
		m.chats = msg.chats
		m.allChats = msg.chats
		m.myOpenID = getMyOpenID()
		var cmds []tea.Cmd
		for _, c := range m.chats {
			if c.ID != "" {
				cmds = append(cmds, loadLastMsg(c.ID))
			}
		}
		return m, tea.Batch(cmds...)
	case lastMsgLoaded:
		for i := range m.chats {
			if m.chats[i].ID == msg.chatID && msg.lastMsg != "" {
				m.chats[i].LastMsg = msg.lastMsg
				m.chats[i].LastTime = msg.lastTime
			}
		}
		for i := range m.allChats {
			if m.allChats[i].ID == msg.chatID && msg.lastMsg != "" {
				m.allChats[i].LastMsg = msg.lastMsg
				m.allChats[i].LastTime = msg.lastTime
			}
		}
	case msgsLoaded:
		m.loadingMsgs = false
		if msg.chatID == m.activeChatID {
			m.msgs = msg.msgs
			// auto scroll to bottom
			m.msgScroll = 999999
		}
	case msgSent:
		return m, loadMsgs(msg.chatID)
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			leftW := m.w * 35 / 100
			if leftW < 28 { leftW = 28 }
			if msg.X < leftW {
				// Clicked left panel
				if m.focus != focusChatList {
					m.focus = focusChatList
					m.input.Blur()
					m.search.Blur()
				}
				// Calculate which chat was clicked (y=0 is top border, y=1 search, items start at y=2)
				clickY := msg.Y - 2 // offset for border + search
				if clickY >= 0 {
					idx := m.chatScroll + clickY/2
					if idx >= 0 && idx < len(m.chats) {
						m.chatIdx = idx
						// Double-purpose: click opens the chat
						c := m.chats[m.chatIdx]
						m.activeChatID, m.activeChatName = c.ID, c.Name
						m.loadingMsgs = true
						m.focus = focusMessages
						m.msgScroll = 0
						return m, loadMsgs(c.ID)
					}
				}
			} else {
				// Clicked right panel
				// Check if click is in the input area (bottom 3 rows)
				msgH := m.h - 6
				if msg.Y > msgH+1 {
					m.focus = focusInput
					m.input.Focus()
				} else {
					m.focus = focusMessages
					m.input.Blur()
				}
			}
		}
	}
	// Always update spinner when loading
	if m.loading || m.loadingMsgs {
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}

	if m.focus == focusInput {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	if m.focus == focusSearch {
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.focus {
	case focusInput:
		switch k {
		case "enter":
			if t := m.input.Value(); t != "" && m.activeChatID != "" {
				m.input.SetValue("")
				return m, doSend(m.activeChatID, t)
			}
		case "esc":
			m.focus = focusMessages
			m.input.Blur()
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	case focusSearch:
		switch k {
		case "enter":
			if q := strings.ToLower(m.search.Value()); q != "" {
				var f []Chat
				for _, c := range m.allChats {
					if strings.Contains(strings.ToLower(c.Name), q) {
						f = append(f, c)
					}
				}
				f = append(f, searchUsers(q)...)
				m.chats, m.chatIdx, m.chatScroll = f, 0, 0
			}
			m.focus = focusChatList
			m.search.Blur()
		case "esc":
			m.searching = false
			m.chats, m.chatIdx, m.chatScroll = m.allChats, 0, 0
			m.focus = focusChatList
			m.search.Blur()
			m.search.SetValue("")
		default:
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			return m, cmd
		}
	case focusMessages:
		switch k {
		case "j", "down":
			m.msgScroll++
		case "k", "up":
			if m.msgScroll > 0 {
				m.msgScroll--
			}
		case "i", "enter":
			m.focus = focusInput
			m.input.Focus()
		case "esc", "h", "left":
			m.focus = focusChatList
		case "q":
			return m, tea.Quit
		case "r":
			if m.activeChatID != "" {
				m.loadingMsgs = true
				return m, loadMsgs(m.activeChatID)
			}
		}
	case focusChatList:
		switch k {
		case "q":
			return m, tea.Quit
		case "j", "down":
			if m.chatIdx < len(m.chats)-1 {
				m.chatIdx++
				m.ensureVisible()
			}
		case "k", "up":
			if m.chatIdx > 0 {
				m.chatIdx--
				m.ensureVisible()
			}
		case "enter", "l", "right":
			if m.chatIdx < len(m.chats) {
				c := m.chats[m.chatIdx]
				m.activeChatID, m.activeChatName = c.ID, c.Name
				m.loadingMsgs = true
				m.focus = focusMessages
				m.msgScroll = 0
				return m, loadMsgs(c.ID)
			}
		case "/":
			m.focus, m.searching = focusSearch, true
			m.search.Focus()
			m.search.SetValue("")
		case "esc":
			if m.searching {
				m.searching = false
				m.chats, m.chatIdx, m.chatScroll = m.allChats, 0, 0
				m.search.SetValue("")
			}
		case "r":
			m.loading = true
			return m, loadChats
		}
	}
	return m, nil
}

func (m *model) ensureVisible() {
	vis := (m.h - 6) / 2
	if m.chatIdx < m.chatScroll {
		m.chatScroll = m.chatIdx
	}
	if m.chatIdx >= m.chatScroll+vis {
		m.chatScroll = m.chatIdx - vis + 1
	}
}

// --- view ---

func (m model) View() string {
	if m.w == 0 {
		return ""
	}

	leftW := m.w*35/100
	if leftW < 28 { leftW = 28 }
	rightW := m.w - leftW

	// inner width = total - 2 (border)
	leftInner := leftW - 2
	rightInner := rightW - 2
	panelH := m.h - 4 // total = panelH+2 (box) + 1 (newline) + 1 (help) = m.h

	// --- Left panel content ---
	var leftLines []string
	if m.searching {
		m.search.Width = leftInner - 2
		leftLines = append(leftLines, " "+m.search.View())
	} else {
		leftLines = append(leftLines, dimStyle.Render(" / search..."))
	}

	if m.loading {
		leftLines = append(leftLines, " "+m.spin.View()+" loading...")
	} else {
		vis := (m.h - 6) / 2
		end := min(m.chatScroll+vis, len(m.chats))
		for i := m.chatScroll; i < end; i++ {
			c := m.chats[i]
			sel := i == m.chatIdx
			dot := blueDot
			if c.Mode == "p2p" { dot = greenDot }

			ts := formatTime(c.LastTime)
			name := c.Name
			// Truncate name if needed
			maxNameW := leftInner - lipgloss.Width(ts) - 5
			if lipgloss.Width(name) > maxNameW {
				name = truncStr(name, maxNameW)
			}

			nr := nameStyle.Render(name)
			if sel { nr = selStyle.Render(name) }

			l1 := fmt.Sprintf(" %s %s", dot, nr)
			// Pad between name and time
			pad := leftInner - lipgloss.Width(l1) - lipgloss.Width(ts)
			if pad < 1 { pad = 1 }
			l1 += strings.Repeat(" ", pad) + dimStyle.Render(ts)

			lastMsg := c.LastMsg
			if lipgloss.Width(lastMsg) > leftInner-4 {
				lastMsg = truncStr(lastMsg, leftInner-4)
			}
			l2 := "   " + dimStyle.Render(lastMsg)

			leftLines = append(leftLines, l1, l2)
		}
	}

	leftContent := strings.Join(leftLines, "\n")

	// --- Right panel: messages ---
	inputBoxH := 3 // border + content + border
	msgH := panelH - inputBoxH - 2 // minus input box height and its borders
	var msgLines []string
	if m.loadingMsgs {
		msgLines = append(msgLines, " "+m.spin.View()+" loading...")
	} else if len(m.msgs) == 0 && m.activeChatID == "" {
		msgLines = append(msgLines, dimStyle.Render(" select a chat"))
	} else {
		for _, msg := range m.msgs {
			isMe := msg.SenderID == m.myOpenID
			c := cleanContent(msg.Content, msg.MsgType)
			switch msg.MsgType {
			case "system":
				msgLines = append(msgLines, dimStyle.Render(" --- "+c+" ---"), "")
				continue
			case "image":
				c = "[image]"
			case "file":
				c = "[file]"
			case "interactive":
				c = "[card]"
			}
			bar := blueBar
			if isMe { bar = greenBar }
			msgLines = append(msgLines, fmt.Sprintf(" %s %s  %s", bar, nameStyle.Render(msg.Sender), dimStyle.Render(formatTime(msg.Time))))
			for _, wl := range wrapText(c, rightInner-5) {
				msgLines = append(msgLines, " "+bar+" "+wl)
			}
			msgLines = append(msgLines, "")
		}
	}

	// Scroll: show last msgH lines
	if len(msgLines) > msgH {
		start := len(msgLines) - msgH
		if m.msgScroll < start {
			start = max(0, m.msgScroll)
		}
		end := start + msgH
		if end > len(msgLines) { end = len(msgLines) }
		msgLines = msgLines[start:end]
	}
	msgContent := strings.Join(msgLines, "\n")

	// Input
	m.input.Width = rightInner - 4
	inputContent := " > " + m.input.View()

	// --- Render boxes ---
	leftActive := m.focus == focusChatList || m.focus == focusSearch
	leftPanel := renderBox(leftInner, panelH, leftContent, "Chats", leftActive)

	rightTitle := m.activeChatName
	if rightTitle == "" { rightTitle = "Messages" }
	rightActive := m.focus == focusMessages || m.focus == focusInput
	msgPanel := renderBox(rightInner, panelH-inputBoxH-1, msgContent, rightTitle, rightActive)
	inputPanel := renderBox(rightInner, 1, inputContent, "", rightActive)
	rightPanel := msgPanel + "\n" + inputPanel

	main := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
	return main + "\n" + m.helpBar()
}

// renderBox draws a rounded border box with optional title in the top border.
func renderBox(innerW, innerH int, content, title string, active bool) string {
	bc := dim
	if active { bc = blue }
	brd := lipgloss.NewStyle().Foreground(bc)

	// Top border: ╭─ Title ────╮
	topLine := brd.Render("╭")
	if title != "" {
		var t string
		if active {
			t = titleActive.Render(" " + title + " ")
		} else {
			t = titleInact.Render(" " + title + " ")
		}
		topLine += t
		fillW := innerW - lipgloss.Width(t)
		if fillW > 0 {
			topLine += brd.Render(strings.Repeat("─", fillW))
		}
	} else {
		topLine += brd.Render(strings.Repeat("─", innerW))
	}
	topLine += brd.Render("╮")

	// Content lines, padded to innerW
	contentLines := strings.Split(content, "\n")
	// Pad or truncate to innerH
	for len(contentLines) < innerH {
		contentLines = append(contentLines, "")
	}
	contentLines = contentLines[:innerH]

	var body []string
	for _, cl := range contentLines {
		w := lipgloss.Width(cl)
		pad := innerW - w
		if pad < 0 { pad = 0 }
		body = append(body, brd.Render("│")+cl+strings.Repeat(" ", pad)+brd.Render("│"))
	}

	// Bottom border
	botLine := brd.Render("╰") + brd.Render(strings.Repeat("─", innerW)) + brd.Render("╯")

	lines := []string{topLine}
	lines = append(lines, body...)
	lines = append(lines, botLine)
	return strings.Join(lines, "\n")
}

func (m model) helpBar() string {
	var parts []string
	add := func(k, d string) { parts = append(parts, helpKey.Render(k)+" "+dimStyle.Render(d)) }
	switch m.focus {
	case focusChatList:
		add("q", "quit"); add("j/k", "nav"); add("enter", "open"); add("/", "search"); add("r", "refresh")
	case focusMessages:
		add("esc", "back"); add("j/k", "scroll"); add("i", "input"); add("r", "refresh")
	case focusInput:
		add("enter", "send"); add("esc", "back")
	case focusSearch:
		add("enter", "search"); add("esc", "cancel")
	}
	return " " + strings.Join(parts, "  ")
}

// --- helpers ---

// cleanContent strips HTML tags, normalizes image refs, etc.
func cleanContent(s string, msgType string) string {
	// Strip HTML tags
	result := []byte{}
	inTag := false
	for i := 0; i < len(s); i++ {
		if s[i] == '<' {
			inTag = true
			continue
		}
		if s[i] == '>' && inTag {
			inTag = false
			continue
		}
		if !inTag {
			result = append(result, s[i])
		}
	}
	s = string(result)

	// Replace [Image: ...] with [image]
	for {
		idx := strings.Index(s, "[Image:")
		if idx < 0 { break }
		end := strings.Index(s[idx:], "]")
		if end < 0 { break }
		s = s[:idx] + "[image]" + s[idx+end+1:]
	}

	// Trim whitespace
	s = strings.TrimSpace(s)
	return s
}

func truncStr(s string, w int) string {
	cur := 0
	for i, r := range s {
		rw := lipgloss.Width(string(r))
		if cur+rw > w { return s[:i] }
		cur += rw
	}
	return s
}

func wrapText(s string, w int) []string {
	if w <= 0 { w = 40 }
	var lines []string
	cur, start := 0, 0
	runes := []rune(s)
	for i, r := range runes {
		rw := lipgloss.Width(string(r))
		if cur+rw > w && i > start {
			lines = append(lines, string(runes[start:i]))
			start, cur = i, 0
		}
		cur += rw
	}
	if start < len(runes) { lines = append(lines, string(runes[start:])) }
	if len(lines) == 0 { lines = []string{""} }
	return lines
}

func min(a, b int) int { if a < b { return a }; return b }
func max(a, b int) int { if a > b { return a }; return b }

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
