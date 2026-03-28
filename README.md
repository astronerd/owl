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
- Node.js (for installing lark-cli)
- Python 3 (for user token management / message sending)
- A terminal with Kitty graphics protocol support (Ghostty, Kitty, WezTerm) for image preview

## Setup

### 1. Install and configure lark-cli

[lark-cli](https://github.com/larksuite/cli) is the official Feishu/Lark CLI tool. owl uses it for all API calls.

```bash
npm install -g @larksuite/cli
```

Initialize and login:

```bash
lark-cli config init
lark-cli auth login --recommend
```

### 2. Create a Feishu self-built app

Go to [Feishu Open Platform](https://open.feishu.cn/app) and create a new app. Then add the following permissions under **Permissions & Scopes**:

| Permission | Description |
|-----------|-------------|
| `im:message` | Read and send messages |
| `im:message:readonly` | Read message content |
| `im:message.send_as_user` | Send messages as user identity |
| `im:chat:read` | Read chat list |
| `im:resource` | Download images and files |
| `contact:user.search:readonly` | Search contacts |
| `offline_access` | Long-lived refresh token |

After adding permissions, go to your app's **Security Settings** and add the redirect URL:

```
http://localhost:19980/callback
```

Then publish the app version and have your admin approve the permissions.

### 3. Clone and build

```bash
git clone https://github.com/astronerd/owl.git
cd owl
go build -o owl .
```

### 4. Configure credentials

```bash
cp .env.example .env
# Edit .env with your App ID and App Secret from the Feishu app dashboard
```

### 5. Login for message sending

The first time, run the OAuth login flow (token auto-refreshes after this):

```bash
cd ~/owl  # the Python helper directory
FEISHU_APP_ID=xxx FEISHU_APP_SECRET=xxx python3 app.py --login
```

### 6. Run

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
