# feishu-tui

Terminal client for Feishu/Lark, built with Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea).

Read and send messages as yourself, directly from the terminal.

![Go](https://img.shields.io/badge/Go-1.23+-blue)
![License](https://img.shields.io/badge/License-MIT-green)

## Features

- Group chats and P2P conversations in one list, sorted by recent activity
- Send messages as your own identity (not a bot)
- Search chats and contacts
- Pin/unpin conversations locally
- Keyboard-driven with mouse support
- Lazygit-style transparent TUI

## Prerequisites

- Go 1.23+
- [lark-cli](https://github.com/larksuite/cli) installed and configured (`npm install -g @larksuite/cli`)
- Python 3 (for user token management / message sending)
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
git clone https://github.com/gengdawei/feishu-tui.git
cd feishu-tui
go build -o feishu-tui .
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
cd ~/feishu-tui  # the Python helper directory
FEISHU_APP_ID=xxx FEISHU_APP_SECRET=xxx python3 app.py --login
```

5. Run:

```bash
FEISHU_APP_ID=xxx FEISHU_APP_SECRET=xxx ./feishu-tui
```

## Keybindings

### Chat list
| Key | Action |
|-----|--------|
| `j/k` | Navigate up/down |
| `Enter` | Open chat |
| `/` | Search chats & contacts |
| `r` | Refresh |
| `q` | Quit |

### Messages
| Key | Action |
|-----|--------|
| `j/k` | Scroll messages |
| `i` / `Enter` | Start typing |
| `Esc` | Back to chat list |
| `r` | Refresh messages |

### Input
| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Esc` | Cancel input |

## Architecture

- **Go + Bubble Tea**: TUI rendering and interaction
- **lark-cli**: Feishu API calls for reading chats and messages (via `--as user`)
- **Python helper** (`~/feishu-tui/feishu_api.py`): OAuth token management and message sending (works around lark-cli's `api POST --as user` bug)

## License

MIT
