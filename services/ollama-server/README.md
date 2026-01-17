# Ollama Server

Local LLM server running on x86_64 Debian with NVIDIA GPU acceleration.

## Current Setup

- **Host**: lenovo (192.168.8.230)
- **GPU**: NVIDIA RTX 2070 Mobile (8GB VRAM)
- **OS**: Debian 12 (Bookworm)
- **API**: http://192.168.8.230:11434

## Models

| Model | Size | Use Case |
|-------|------|----------|
| `qwen2.5-coder:7b` | 4.7GB | Code generation, best small coding model |
| `llama3.2:3b` | 2.0GB | General chat, fast responses |

## Usage

### API Endpoints

```bash
# Generate text
curl http://192.168.8.230:11434/api/generate \
  -d '{"model": "qwen2.5-coder:7b", "prompt": "Write a Python hello world"}'

# Chat
curl http://192.168.8.230:11434/api/chat \
  -d '{"model": "llama3.2", "messages": [{"role": "user", "content": "Hello"}]}'

# List models
curl http://192.168.8.230:11434/api/tags
```

### Pull New Models

```bash
ssh tim@192.168.8.230 "ollama pull <model-name>"
```

## Setup New Server

Run the setup script on a fresh Debian 12 install:

```bash
curl -fsSL https://raw.githubusercontent.com/timholm/HolmOS/main/services/ollama-server/setup.sh | sudo bash
```

Or manually:

```bash
sudo bash setup.sh
sudo reboot  # Required for NVIDIA drivers
```

## Management

```bash
# Check status
ssh tim@192.168.8.230 "systemctl status ollama"

# View logs
ssh tim@192.168.8.230 "journalctl -u ollama -f"

# Check GPU
ssh tim@192.168.8.230 "nvidia-smi"

# Restart service
ssh tim@192.168.8.230 "sudo systemctl restart ollama"
```

## Configuration

Ollama listens on all interfaces via systemd override:

```
/etc/systemd/system/ollama.service.d/override.conf
```

Lid close sleep is disabled via:

```
/etc/systemd/logind.conf (HandleLidSwitch=ignore)
```
