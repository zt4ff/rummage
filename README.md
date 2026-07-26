# Rummage

Automated research agents that search the web, extract insights with LLMs, and merge findings — all on a schedule you define.

## Quick Start

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose
- [pnpm](https://pnpm.io/installation)
- A [Groq API key](https://console.groq.com) (free tier available)

### 1. Clone and configure

```bash
git clone https://github.com/<your-username>/rummage.git
cd rummage
cp .env.sample .env
```

Edit `.env` and add your Groq API key:

```
GROQ_API_KEY=gsk_xxxxxxxxxxxxx
```

### 2. Start the database and backend

```bash
docker compose up --build -d
```

This starts PostgreSQL and the Go API server on `http://localhost:3001`.

### 3. Start the frontend

```bash
cd frontend
pnpm install
pnpm dev
```

The dashboard is now running at `http://localhost:3000`.

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `GROQ_API_KEY` | Yes | — | API key for LLM inference ([console.groq.com](https://console.groq.com)) |
| `MODEL` | No | `llama-3.3-70b-versatile` | Model identifier for Groq |
| `TAVILY_API_KEY` | No | — | API key for Tavily search. Without it, search falls back to DuckDuckGo, SearXNG, and Google ([app.tavily.com](https://app.tavily.com)) |
| `PORT` | No | `3001` | Backend server port |
| `DATABASE_URL` | No | `postgres://localhost:5432/scaff?sslmode=disable` | PostgreSQL connection string (overridden by Docker Compose) |

## Usage

1. Open the dashboard at `http://localhost:3000`
2. Create a **topic** with a name, sources to research, and a cron schedule (e.g. `0 9 * * *` for daily at 9am)
3. Rummage spawns multiple research agents that search the web, fetch pages, and send content to the LLM for structured extraction
4. Results are merged and stored — view them in the dashboard

You can also trigger an immediate run from the topic detail page.

## API

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/topics` | List all topics |
| `POST` | `/api/topics` | Create a topic |
| `GET` | `/api/topics/:id` | Get a topic |
| `DELETE` | `/api/topics/:id` | Delete a topic |
| `POST` | `/api/topics/:id/run-now` | Trigger an immediate run |
| `GET` | `/api/topics/:id/runs` | List runs for a topic |
| `GET` | `/api/runs/:id` | Get a run with its result |
| `GET` | `/api/prompts/:name` | View a prompt template |

## License

MIT
# rummage
