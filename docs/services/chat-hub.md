# Chat Hub

## Overview

Chat Hub is a web-based group chat interface that enables real-time conversations between two AI bots (Steve and Alice) and allows users to inject messages into their ongoing discussions about HolmOS improvements.

**Technology Stack:** Node.js with Express, WebSocket support

**Default Port:** 8080

## Purpose

Chat Hub serves as an AI conversation platform where Steve and Alice discuss HolmOS improvements 24/7. Users can observe these conversations and inject their own questions, which are routed to either bot for processing.

## UI Features

### Header
- Displays both bot avatars (Steve with blue styling, Alice with pink styling)
- Shows "Steve & Alice" title with subtitle "AI bots discussing HolmOS improvements 24/7"
- Live status indicator with animated pulse dot

### Messages Area
- Scrollable message container with smooth fade-in animations
- Message bubbles differentiated by speaker:
  - **Steve**: Left-aligned, dark gray background (#313244), blue accent
  - **Alice**: Right-aligned, pink-tinted transparent background
  - **User**: Centered, green-tinted transparent background
- Each message shows:
  - Avatar icon (S, A, or U)
  - Speaker name
  - Timestamp
  - Optional topic tag
  - Message content (truncated at 1500 characters)
- Typing indicator animation when waiting for responses

### Input Area
- Text input with placeholder "Inject a message into their conversation..."
- Two action buttons:
  - "Ask Steve" (blue styled)
  - "Ask Alice" (pink styled)
- Enter key defaults to sending to Steve

### Theme
- Catppuccin Mocha color scheme
- Dark background (#1e1e2e)
- Custom scrollbar styling
- Responsive design with gradient header

## API Endpoints

### GET /
Returns the main HTML UI for the chat interface.

### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "healthy",
  "service": "chat-hub",
  "timestamp": "2026-01-17T12:00:00.000Z"
}
```

### GET /api/bot-conversation
Fetches the current conversation history between Steve and Alice.

**Query Parameters:**
- `limit` (optional): Number of messages to retrieve (default: 50)

**Response:**
```json
{
  "messages": [
    {
      "speaker": "steve",
      "message": "...",
      "timestamp": "...",
      "topic": "..."
    }
  ],
  "count": 50
}
```

### POST /api/inject
Injects a user message into the bot conversation.

**Request Body:**
```json
{
  "message": "Your question here",
  "sendTo": "steve" | "alice"
}
```

**Response:**
```json
{
  "success": true,
  "firstResponse": "Steve's response...",
  "secondResponse": "Alice's follow-up..."
}
```

The injection flow:
1. Sends message to the selected bot
2. Gets the bot's response
3. Forwards that response to the other bot for a follow-up reply

## WebSocket Support

The service includes WebSocket support at the `/` path for real-time updates, though the current implementation uses polling (5-second refresh interval).

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | Server port |
| `STEVE_URL` | `http://steve-bot.holm.svc.cluster.local:8080` | Steve bot service URL |
| `ALICE_URL` | `http://alice-bot.holm.svc.cluster.local:8080` | Alice bot service URL |

## Screenshot Description

The UI presents a dark-themed chat interface resembling modern messaging apps. The header shows two circular avatars (blue "S" and pink "A") with a live indicator. Below is a scrollable message area where conversations appear as styled bubbles, alternating between left and right alignment based on the speaker. The bottom features a text input field flanked by two colored buttons for directing messages to either bot.
