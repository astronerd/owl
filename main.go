package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

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

	redDot   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render("●")
	greenDot = lipgloss.NewStyle().Foreground(green).Render("●")
	blueDot  = lipgloss.NewStyle().Foreground(blue).Render("●")
	greenBar = lipgloss.NewStyle().Foreground(green).Render("│")
	blueBar  = lipgloss.NewStyle().Foreground(blue).Render("│")
)

// Package-level state set by View(), read by Update() for click handling.
// View() is a value receiver so model mutations there don't persist.
var (
	viewMsgLineLinks []string
	viewPreviewStr   string
)

type focus int

const (
	focusChatList focus = iota
	focusMessages
	focusInput
	focusSearch
	focusImagePreview
)

// --- tea messages ---

type chatsLoaded struct{ chats []Chat }
type msgsLoaded struct {
	chatID  string
	msgs    []Message
	isOpen  bool // true when user explicitly opened chat (not auto-refresh)
}
type msgSent struct{ chatID string }
type lastMsgLoaded struct {
	chatID, lastMsg, lastTime, lastMsgID string
}
type tickMsg time.Time
type imageReady struct {
	messageID string
	path      string
}
type mergeForwardLoaded struct {
	messageID string
	msgs      []Message
}
type docTitleReady struct {
	token, title string
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
	lastSeenMsgID  map[string]string // chatID -> msgID when user last opened
	hasNewMsg      map[string]bool   // chatID -> has unread
	tickCount      int
	imgCache       map[string]string     // messageID -> file path
	imgLoading     map[string]bool       // messageID -> downloading
	mergeCache     map[string][]Message  // messageID -> sub-messages
	mergeLoading   map[string]bool       // messageID -> loading
	docTitleCache  map[string]string     // docToken -> title
	docTitleLoading map[string]bool
	previewImgPath string                // image path for full preview
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
	return model{
		input: ti, search: si, spin: sp, loading: true,
		lastSeenMsgID: make(map[string]string),
		hasNewMsg:     make(map[string]bool),
		imgCache:        make(map[string]string),
		imgLoading:      make(map[string]bool),
		mergeCache:      make(map[string][]Message),
		mergeLoading:    make(map[string]bool),
		docTitleCache:   make(map[string]string),
		docTitleLoading: make(map[string]bool),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(loadChats, tea.WindowSize(), tea.EnableMouseAllMotion, m.spin.Tick,
		tea.Tick(10*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }))
}

func loadChats() tea.Msg {
	myID := getMyOpenID()
	return chatsLoaded{chats: append(getP2PChats(myID, 10), getChatList(30)...)}
}

func loadMsgs(id string, isOpen bool) tea.Cmd {
	return func() tea.Msg { return msgsLoaded{id, getMessagesPaged(id, 30), isOpen} }
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
			return lastMsgLoaded{id, p, m.Time, m.ID}
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
	case tickMsg:
		m.tickCount++
		cmds := []tea.Cmd{
			tea.Tick(10*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
		}
		// Auto-refresh active chat messages every tick (5s)
		if m.activeChatID != "" && !m.loadingMsgs {
			cmds = append(cmds, loadMsgs(m.activeChatID, false))
		}
		// Refresh last messages for badge detection every 6th tick (30s)
		if m.tickCount%6 == 0 {
			for _, c := range m.chats {
				if c.ID != "" {
					cmds = append(cmds, loadLastMsg(c.ID))
				}
			}
		}
		return m, tea.Batch(cmds...)
	case lastMsgLoaded:
		for i := range m.chats {
			if m.chats[i].ID == msg.chatID && msg.lastMsg != "" {
				// Badge: new message if msgID changed and chat is not currently active
				if msg.lastMsgID != "" && m.chats[i].LastMsgID != "" &&
					msg.lastMsgID != m.chats[i].LastMsgID &&
					msg.chatID != m.activeChatID {
					m.hasNewMsg[msg.chatID] = true
				}
				m.chats[i].LastMsg = msg.lastMsg
				m.chats[i].LastTime = msg.lastTime
				m.chats[i].LastMsgID = msg.lastMsgID
			}
		}
		for i := range m.allChats {
			if m.allChats[i].ID == msg.chatID && msg.lastMsg != "" {
				if msg.lastMsgID != "" && m.allChats[i].LastMsgID != "" &&
					msg.lastMsgID != m.allChats[i].LastMsgID &&
					msg.chatID != m.activeChatID {
					m.hasNewMsg[msg.chatID] = true
				}
				m.allChats[i].LastMsg = msg.lastMsg
				m.allChats[i].LastTime = msg.lastTime
				m.allChats[i].LastMsgID = msg.lastMsgID
			}
		}
	case msgsLoaded:
		m.loadingMsgs = false
		if msg.chatID == m.activeChatID {
			updated := false
			if msg.isOpen {
				m.msgs = mergeMessages(nil, msg.msgs)
				m.msgScroll = 0
				updated = true
			} else if len(msg.msgs) > 0 {
				newLastID := msg.msgs[len(msg.msgs)-1].ID
				oldLastID := ""
				if len(m.msgs) > 0 {
					oldLastID = m.msgs[len(m.msgs)-1].ID
				}
				if newLastID != oldLastID {
					m.msgs = mergeMessages(m.msgs, msg.msgs)
					updated = true
				}
			}
			delete(m.hasNewMsg, msg.chatID)
			// Fire async downloads for images and merge forwards
			if updated {
				var cmds []tea.Cmd
				for _, mg := range m.msgs {
					if mg.MsgType == "image" && m.imgCache[mg.ID] == "" && !m.imgLoading[mg.ID] {
						m.imgLoading[mg.ID] = true
						mid, ik := mg.ID, parseImageKey(mg.Content)
						if ik != "" {
							cmds = append(cmds, func() tea.Msg {
								path, _ := downloadImage(mid, ik)
								return imageReady{mid, path}
							})
						}
					}
					if mg.MsgType == "merge_forward" && m.mergeCache[mg.ID] == nil && !m.mergeLoading[mg.ID] {
						m.mergeLoading[mg.ID] = true
						mid := mg.ID
						cmds = append(cmds, func() tea.Msg {
							return mergeForwardLoaded{mid, getMergeForwardMessages(mid)}
						})
					}
					// Fetch doc titles from URLs in content
					c := cleanContent(mg.Content, mg.MsgType)
					for _, u := range urlRe.FindAllString(c, -1) {
						token, docType := extractDocToken(u)
						if token != "" && docType != "" && m.docTitleCache[token] == "" && !m.docTitleLoading[token] {
							m.docTitleLoading[token] = true
							tk, dt := token, docType
							cmds = append(cmds, func() tea.Msg {
								title := getDocTitle(tk, dt)
								return docTitleReady{tk, title}
							})
						}
					}
				}
				if len(cmds) > 0 {
					return m, tea.Batch(cmds...)
				}
			}
		}
	case imageReady:
		m.imgLoading[msg.messageID] = false
		if msg.path != "" {
			m.imgCache[msg.messageID] = msg.path
			// Auto-open preview if user clicked this image
			if m.previewImgPath == "pending:"+msg.messageID {
				m.previewImgPath = msg.path
				viewPreviewStr = ""
				m.focus = focusImagePreview
			}
		}
	case mergeForwardLoaded:
		m.mergeLoading[msg.messageID] = false
		if msg.msgs != nil {
			m.mergeCache[msg.messageID] = msg.msgs
		}
	case docTitleReady:
		m.docTitleLoading[msg.token] = false
		if msg.title != "" {
			m.docTitleCache[msg.token] = msg.title
		}
	case msgSent:
		return m, loadMsgs(msg.chatID, false)
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
						return m, loadMsgs(c.ID, true)
					}
				}
			} else {
				// Clicked right panel
				if msg.Y >= m.h-5 {
					m.focus = focusInput
					m.input.Focus()
				} else {
					m.focus = focusMessages
					m.input.Blur()
					lineIdx := msg.Y - 1
					// Find link: exact match first, then ±1
					var u string
					for _, li := range []int{lineIdx, lineIdx - 1, lineIdx + 1} {
						if li >= 0 && li < len(viewMsgLineLinks) && viewMsgLineLinks[li] != "" {
							u = viewMsgLineLinks[li]
							break
						}
					}
					if u != "" {
						if strings.HasPrefix(u, "img:") {
							parts := strings.SplitN(strings.TrimPrefix(u, "img:"), ":", 2)
							msgID := parts[0]
							imgKey := ""
							if len(parts) > 1 {
								imgKey = parts[1]
							}
							m.focus = focusImagePreview
							viewPreviewStr = ""
							if path := m.imgCache[msgID]; path != "" {
								m.previewImgPath = path
							} else {
								m.previewImgPath = "pending:" + msgID
								if imgKey != "" && !m.imgLoading[msgID] {
									m.imgLoading[msgID] = true
									mid, ik := msgID, imgKey
									return m, tea.Batch(m.spin.Tick, func() tea.Msg {
										path, _ := downloadImage(mid, ik)
										return imageReady{mid, path}
									})
								}
							}
						} else if u != "" {
							exec.Command("open", u).Start()
						}
					}
				}
			}
		}
	}
	// Always update spinner when loading
	if m.loading || m.loadingMsgs || (m.focus == focusImagePreview && strings.HasPrefix(m.previewImgPath, "pending:")) {
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
	case focusImagePreview:
		if k == "esc" || k == "q" {
			m.focus = focusMessages
			m.previewImgPath = ""
			viewPreviewStr = ""
		}
		return m, nil
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
		case "up", "k":
			m.msgScroll++
		case "down", "j":
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
				return m, loadMsgs(m.activeChatID, false)
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
				return m, loadMsgs(c.ID, true)
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
			if m.hasNewMsg[c.ID] { dot = redDot }

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
	// Left total = panelH + 2 (renderBox adds 2 for borders)
	// Right = msgBox(msgH+2) + inputBox(1+2=3) = msgH+5 = panelH+2 → msgH = panelH-3
	msgH := panelH - 3
	var msgLines []string
	var msgLineURLs []string // parallel: URL for each line (empty = no link)
	addLine := func(line, url string) {
		msgLines = append(msgLines, line)
		msgLineURLs = append(msgLineURLs, url)
	}
	if m.loadingMsgs {
		addLine(" "+m.spin.View()+" loading...", "")
	} else if len(m.msgs) == 0 && m.activeChatID == "" {
		addLine(dimStyle.Render(" select a chat"), "")
	} else {
		for _, msg := range m.msgs {
			isMe := msg.SenderID == m.myOpenID
			c := cleanContent(msg.Content, msg.MsgType)
			switch msg.MsgType {
			case "system":
				addLine(dimStyle.Render(" --- "+c+" ---"), "")
				addLine("", "")
				continue
			case "file":
				c = "[file]"
			case "interactive":
				c = "[card]"
			case "image", "merge_forward":
				// handled below
			}
			bar := blueBar
			if isMe { bar = greenBar }
			addLine(fmt.Sprintf(" %s %s  %s", bar, nameStyle.Render(msg.Sender), dimStyle.Render(formatTime(msg.Time))), "")

			if msg.MsgType == "image" {
				imgLink := "img:" + msg.ID + ":" + parseImageKey(msg.Content)
				if m.imgLoading[msg.ID] {
					addLine(" "+bar+" "+dimStyle.Render("[image: loading...]"), imgLink)
				} else {
					addLine(" "+bar+" "+linkPill.Render("[image]"), imgLink)
				}
			} else if msg.MsgType == "merge_forward" {
				if subs := m.mergeCache[msg.ID]; len(subs) > 0 {
					addLine(" "+bar+" "+dimStyle.Render("── forwarded ──"), "")
					for _, sub := range subs {
						sc := cleanContent(sub.Content, sub.MsgType)
						if sub.MsgType == "image" {
							sc = "[image]"
						}
						styled, u := linkifyLine(sc, m.docTitleCache)
						addLine(" "+bar+"  "+nameStyle.Render(sub.Sender)+": "+styled, u)
					}
					addLine(" "+bar+" "+dimStyle.Render("───────────────"), "")
				} else if m.mergeLoading[msg.ID] {
					addLine(" "+bar+" "+dimStyle.Render("[forwarded: loading...]"), "")
				} else {
					addLine(" "+bar+" "+dimStyle.Render("[forwarded messages]"), "")
				}
			} else {
				// Extract inline image keys from raw content before cleaning
				inlineImgKey := parseImageKey(msg.Content)

				// Replace URLs with pill labels BEFORE wrapping
				var firstURL string
				replaced := urlRe.ReplaceAllStringFunc(c, func(u string) string {
					if firstURL == "" {
						firstURL = u
					}
					return urlLabel(u, m.docTitleCache)
				})
				for _, wl := range wrapText(replaced, rightInner-5) {
					lineLink := firstURL
					styled := wl
					// Style bracket pills like [wiki], [doc: xxx], [image]
					if strings.Contains(wl, "[") {
						styled = linkPillRe.ReplaceAllStringFunc(wl, func(p string) string {
							return linkPill.Render(p)
						})
					}
					// Make [image] clickable with inline image key
					if strings.Contains(wl, "[image]") && inlineImgKey != "" {
						lineLink = "img:" + msg.ID + ":" + inlineImgKey
					}
					addLine(" "+bar+" "+styled, lineLink)
				}
			}
			addLine("", "")
		}
	}

	// Scroll: msgScroll=0 means bottom (newest), higher = further up (older)
	if len(msgLines) > msgH {
		maxScroll := len(msgLines) - msgH
		if m.msgScroll > maxScroll {
			m.msgScroll = maxScroll
		}
		end := len(msgLines) - m.msgScroll
		start := end - msgH
		if start < 0 { start = 0 }
		msgLines = msgLines[start:end]
		msgLineURLs = msgLineURLs[start:end]
	}
	// Ensure links array matches what renderBox will actually display
	if len(msgLineURLs) > msgH {
		msgLineURLs = msgLineURLs[:msgH]
	}
	viewMsgLineLinks = msgLineURLs
	msgContent := strings.Join(msgLines, "\n")

	m.input.Width = rightInner - 4

	// --- Render boxes ---
	leftActive := m.focus == focusChatList || m.focus == focusSearch
	leftPanel := renderBox(leftInner, panelH, leftContent, "Chats", leftActive)

	var msgPanel, inputPanel string
	if m.focus == focusImagePreview {
		var previewContent string
		if strings.HasPrefix(m.previewImgPath, "pending:") {
			// Still downloading — show spinner
			previewContent = "\n\n  " + m.spin.View() + " downloading image..."
		} else if m.previewImgPath != "" {
			if viewPreviewStr == "" {
				s, _ := renderImageKitty(m.previewImgPath, rightInner-2, panelH-2)
				viewPreviewStr = s
			}
			previewContent = viewPreviewStr
		}
		msgPanel = renderBox(rightInner, panelH-2, previewContent, "Image Preview", true)
		inputPanel = renderBox(rightInner, 1, " "+dimStyle.Render("esc to go back"), "", true)
	} else {
		rightTitle := m.activeChatName
		if rightTitle == "" { rightTitle = "Messages" }
		rightActive := m.focus == focusMessages || m.focus == focusInput
		msgPanel = renderBox(rightInner, msgH, msgContent, rightTitle, rightActive)
		inputPanel = renderBox(rightInner, 1, " > "+m.input.View(), "", rightActive)
	}

	// Join left and right line by line to ensure exact alignment
	leftSplit := strings.Split(leftPanel, "\n")
	rightMsgSplit := strings.Split(msgPanel, "\n")
	rightInputSplit := strings.Split(inputPanel, "\n")
	rightSplit := append(rightMsgSplit, rightInputSplit...)

	leftTotalW := leftW // expected visual width of left panel

	var output strings.Builder
	totalLines := max(len(leftSplit), len(rightSplit))
	for i := 0; i < totalLines; i++ {
		l, r := "", ""
		if i < len(leftSplit) {
			l = leftSplit[i]
		}
		if i < len(rightSplit) {
			r = rightSplit[i]
		}
		// Pad left to consistent width
		lw := lipgloss.Width(l)
		if lw < leftTotalW {
			l += strings.Repeat(" ", leftTotalW-lw)
		}
		output.WriteString(l + r + "\n")
	}
	output.WriteString(m.helpBar())
	// Always clear Kitty image overlays before rendering (prevents stale images)
	return "\x1b_Ga=d\x1b\\" + output.String()
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
		add("q", "quit"); add("↑/↓", "nav"); add("enter", "open"); add("/", "search"); add("r", "refresh")
	case focusMessages:
		add("esc", "back"); add("↑/↓", "scroll"); add("i", "input"); add("r", "refresh")
	case focusInput:
		add("enter", "send"); add("esc", "back")
	case focusSearch:
		add("enter", "search"); add("esc", "cancel")
	case focusImagePreview:
		add("esc", "back")
	}
	return " " + strings.Join(parts, "  ")
}

// --- helpers ---

// hrefRe extracts href from <a> tags
var hrefRe = regexp.MustCompile(`<a[^>]+href="([^"]*)"[^>]*>`)

// cleanContent strips HTML tags but preserves link URLs.
func cleanContent(s string, msgType string) string {
	// Convert <a href="url">text</a> → text (url)
	s = hrefRe.ReplaceAllStringFunc(s, func(tag string) string {
		m := hrefRe.FindStringSubmatch(tag)
		if len(m) >= 2 {
			return "FEISHU_LINK_START:" + m[1] + ":"
		}
		return ""
	})
	s = strings.ReplaceAll(s, "</a>", "")

	// Strip remaining HTML tags
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

	// Convert link markers back: FEISHU_LINK_START:url:text → text url
	for {
		idx := strings.Index(s, "FEISHU_LINK_START:")
		if idx < 0 {
			break
		}
		rest := s[idx+len("FEISHU_LINK_START:"):]
		endURL := strings.Index(rest, ":")
		if endURL < 0 {
			break
		}
		url := rest[:endURL]
		s = s[:idx] + url + " " + rest[endURL+1:]
	}

	// Replace [Image: ...] with [image]
	for {
		idx := strings.Index(s, "[Image:")
		if idx < 0 {
			break
		}
		end := strings.Index(s[idx:], "]")
		if end < 0 {
			break
		}
		s = s[:idx] + "[image]" + s[idx+end+1:]
	}

	s = strings.TrimSpace(s)
	return s
}

// linkify replaces URLs with styled pill labels.
var urlRe = regexp.MustCompile(`https?://[^\s<>\]\)]+`)

var linkPill = lipgloss.NewStyle().Foreground(blue).Underline(true)
var linkPillRe = regexp.MustCompile(`\[[^\]]+\]`)

// linkifyLine replaces URLs with styled pills and returns the first URL found.
func linkifyLine(s string, docTitles map[string]string) (string, string) {
	var firstURL string
	result := urlRe.ReplaceAllStringFunc(s, func(u string) string {
		if firstURL == "" {
			firstURL = u
		}
		label := urlLabel(u, docTitles)
		return linkPill.Render(label)
	})
	return result, firstURL
}

// docTokenRe extracts the token from feishu doc URLs
var docTokenRe = regexp.MustCompile(`feishu\.cn/(?:docx|docs|wiki|sheets|base|slides|mindnotes)/([A-Za-z0-9]+)`)

func extractDocToken(u string) (token, docType string) {
	if strings.Contains(u, "/docx/") || strings.Contains(u, "/docs/") {
		docType = "docx"
	} else if strings.Contains(u, "/wiki/") {
		docType = "wiki"
	} else if strings.Contains(u, "/sheets/") || strings.Contains(u, "/base/") {
		docType = "sheet"
	} else if strings.Contains(u, "/slides/") {
		docType = "slides"
	} else if strings.Contains(u, "/mindnotes/") {
		docType = "mindmap"
	}
	m := docTokenRe.FindStringSubmatch(u)
	if len(m) >= 2 {
		token = m[1]
	}
	return
}

func urlLabel(u string, docTitles map[string]string) string {
	token, docType := extractDocToken(u)
	if token != "" && docType != "" {
		if title, ok := docTitles[token]; ok && title != "" {
			return "[" + docType + ": " + title + "]"
		}
		return "[" + docType + "]"
	}
	if strings.Contains(u, "feishu.cn") || strings.Contains(u, "larksuite.com") {
		return "[feishu]"
	}
	parts := strings.SplitN(u, "/", 4)
	if len(parts) >= 3 {
		return "[" + parts[2] + "]"
	}
	return "[link]"
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

// mergeMessages merges new messages with old ones, preserving sender names from old.
func mergeMessages(old, new []Message) []Message {
	if len(old) == 0 {
		return new
	}
	oldByID := make(map[string]Message, len(old))
	for _, m := range old {
		oldByID[m.ID] = m
	}
	for i, m := range new {
		if prev, ok := oldByID[m.ID]; ok {
			if m.Sender == "" && prev.Sender != "" {
				new[i].Sender = prev.Sender
			}
			if m.SenderID == "" && prev.SenderID != "" {
				new[i].SenderID = prev.SenderID
			}
		}
	}
	return new
}


func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
