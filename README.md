# owl

Terminal client for Feishu/Lark, built with Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea).

Read and send messages as yourself, directly from the terminal.

![Go](https://img.shields.io/badge/Go-1.23+-blue)
![License](https://img.shields.io/badge/License-MIT-green)

## Features

- Group chats and P2P conversations in one list, sorted by recent activity
- Send messages as your own identity (not a bot)
- Search chats and contacts
- Auto-refresh messages and new message badges
- Image preview using Kitty graphics protocol (Ghostty, Kitty, WezTerm)
- Clickable document links with title resolution (`[wiki: Doc Title]`)
- Merged forward message expansion
- Keyboard-driven with full mouse support
- Lazygit-style transparent TUI

## Prerequisites

- Go 1.23+
- [lark-cli](https://github.com/larksuite/cli) installed and configured (`npm install -g @larksuite/cli`)
- Python 3 (for user token management / message sending)
- A terminal with Kitty graphics protocol support (Ghostty, Kitty, WezTerm) for image preview
- A Feishu self-built app with the following scopes enabled:
  - `im:message.send_as_user`
  - `im:message`
  - `im:chat:read`
  - `im:message:readonly`
  - `offline_access`
- Redirect URL `http://localhost:19980/callback` configured in your Feishu app's security settings

## Setup

1. Clone and build:

```bash
git clone https://github.com/astronerd/owl.git
cd owl
go build -o owl .
```

2. Configure environment variables:

```bash
cp .env.example .env
# Edit .env with your Feishu app credentials
```

3. Set up lark-cli authentication:

```bash
lark-cli config init
lark-cli auth login --recommend
```

4. Login for message sending (one-time, token auto-refreshes):

```bash
cd ~/owl  # the Python helper directory
FEISHU_APP_ID=xxx FEISHU_APP_SECRET=xxx python3 app.py --login
```

5. Run:

```bash
FEISHU_APP_ID=xxx FEISHU_APP_SECRET=xxx ./owl
```

## Keybindings

### Chat list
| Key | Action |
|-----|--------|
| `↑/↓` | Navigate up/down |
| `Enter` | Open chat |
| `/` | Search chats & contacts |
| `r` | Refresh |
| `q` | Quit |

### Messages
| Key | Action |
|-----|--------|
| `↑/↓` | Scroll messages |
| `i` / `Enter` | Start typing |
| `Esc` | Back to chat list |
| `r` | Refresh messages |
| Click `[image]` | Preview image |
| Click `[doc]` / `[wiki]` | Open in browser |

### Image preview
| Key | Action |
|-----|--------|
| `Esc` | Back to messages |

### Input
| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Esc` | Cancel input |

## Architecture

- **Go + Bubble Tea**: TUI rendering and interaction
- **lark-cli**: Feishu API calls for reading chats and messages (via `--as user`)
- **rasterm**: Kitty graphics protocol for inline image preview
- **Python helper** (`~/owl/feishu_api.py`): OAuth token management and message sending (works around lark-cli's `api POST --as user` bug)

## License

MIT
