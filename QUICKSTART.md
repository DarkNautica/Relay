# Relay — Getting Started on Windows with VS Code + Claude Code

This guide walks you through getting the Relay development environment
running on your Windows machine, ready to build with Claude Code.

---

## Prerequisites

Install these in order:

### 1. Git for Windows
Download from: https://git-scm.com/download/win
Use all defaults during installation.

### 2. Go
Download from: https://go.dev/dl/ — grab the Windows .msi installer.
After install, open a new terminal and verify:
```
go version
```

### 3. Claude Code
Open PowerShell and run:
```powershell
irm https://claude.ai/install.ps1 | iex
```
Close and reopen PowerShell, then verify:
```
claude --version
```

### 4. VS Code
Download from: https://code.visualstudio.com

---

## Setting Up the Project

### 1. Open the relay-server folder in VS Code
```
File → Open Folder → relay/relay-server
```

### 2. Open the integrated terminal
```
Terminal → New Terminal  (or Ctrl + `)
```

### 3. Download Go dependencies
```bash
go mod tidy
```
This downloads gorilla/websocket, gorilla/mux, uuid, and godotenv.

### 4. Create your .env file
```bash
cp .env.example .env
```
Edit .env and set your app key and secret to anything you like for development.

### 5. Run the server
```bash
go run ./main.go
```
You should see:
```
[Relay] Server starting on 0.0.0.0:6001
[Relay] App Key: my-key
[Relay] Dashboard: http://0.0.0.0:6001/dashboard
```

### 6. Test it's running
Open a browser and go to: http://localhost:6001/health
You should see: {"status":"ok"}

---

## Starting Claude Code

In your VS Code terminal, start a Claude Code session:
```bash
claude
```

Claude Code will read the CLAUDE.md file in the project root and understand
the entire architecture before you type a single prompt. From here you can say
things like:

- "Wire up the auth validation in hub.go to use the auth package"
- "Write tests for the channel subscribe flow"
- "Build out the dashboard handler to serve a React app"
- "Add webhook support for connection and disconnection events"

---

## Testing a WebSocket Connection

Once the server is running, you can test it with a browser console:

```javascript
const ws = new WebSocket('ws://localhost:6001/app/my-key')

ws.onmessage = (e) => console.log('Received:', JSON.parse(e.data))

// You should immediately see:
// { event: "relay:connection_established", data: { socket_id: "...", activity_timeout: 120 } }
```

---

## Project Structure Reference

```
relay-server/
├── main.go                     ← Entry point
├── CLAUDE.md                   ← Claude Code context (read this!)
├── .env.example                ← Copy to .env
├── Dockerfile                  ← Docker build
├── docker-compose.yml          ← Local Docker stack
└── internal/
    ├── config/config.go        ← All configuration
    ├── hub/
    │   ├── hub.go              ← Central message broker (THE heart)
    │   ├── client.go           ← Individual WebSocket connection
    │   ├── channel.go          ← Channel management + presence
    │   └── export.go           ← Public API for other packages
    ├── protocol/messages.go    ← All message types and constants
    ├── auth/auth.go            ← HMAC signature validation
    ├── websocket/handler.go    ← HTTP → WebSocket upgrade
    ├── api/handler.go          ← REST API endpoints
    └── server/server.go        ← HTTP server + router
```

---

## Next Steps

In order of priority:

1. `go mod tidy` and make sure everything compiles cleanly
2. Wire the auth package into hub.go validateAuth()
3. Write the first integration test — connect a WebSocket, subscribe to a channel, publish via HTTP, verify receipt
4. Build the dashboard (React app embedded in the binary)
5. Add webhook support
6. Write the relay-js README with usage examples
7. Set up the GitHub organisation and push

---

## Useful Commands

```bash
# Run the server
go run ./main.go

# Build a binary
go build -o relay.exe ./main.go

# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Format all Go code
go fmt ./...

# Check for issues
go vet ./...
```
