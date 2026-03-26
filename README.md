# Envoy + ext_proc → Vertex AI

Envoy sidecar with a Go external processor that:
1. Injects a Vertex AI API key (from AWS Secrets Manager) into every request
2. Injects configurable `safetySettings` into the request body

## Architecture

```
Your App (port 10000)
    → Envoy (ext_proc filter)
        ↕ gRPC (bidirectional stream)
      Go ext_proc sidecar (port 9001)
        ├── Request headers phase: injects ?key=<api_key> into path
        └── Request body phase: injects safetySettings into JSON body
    → Vertex AI (aiplatform.googleapis.com:443, TLS)
```

## How it works

1. Your app sends requests to `http://localhost:10000` with no auth
2. Envoy sends request headers to the ext_proc sidecar over gRPC
3. The sidecar fetches the API key from AWS Secrets Manager (cached with TTL),
   appends it to the `:path` as a query param, and tells envoy to buffer the body
4. Envoy sends the buffered request body to the sidecar
5. The sidecar parses the JSON, injects `safetySettings` if not present,
   and returns the mutated body
6. Envoy forwards the request to Vertex AI over TLS

## Setup

```bash
# Build and run
docker-compose up -d

# Test
curl -s http://localhost:10000/v1/publishers/google/models/gemini-2.5-flash:generateContent \
  -H "Content-Type: application/json" \
  -d '{"contents": [{"role": "user", "parts": [{"text": "hello"}]}]}'
```

## Configuration

All config via environment variables on the `ext-proc` service:

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN_ADDR` | `:9001` | gRPC listen address |
| `SECRET_ID` | `lab/envoy-sds-test` | AWS Secrets Manager secret name |
| `SECRET_JSON_KEY` | `api-key` | Key to extract from secret JSON |
| `SECRET_TTL` | `5m` | How long to cache the secret |
| `VERTEX_SAFETY_HARASSMENT` | `BLOCK_MEDIUM_AND_ABOVE` | Harassment threshold |
| `VERTEX_SAFETY_HATE_SPEECH` | `BLOCK_MEDIUM_AND_ABOVE` | Hate speech threshold |
| `VERTEX_SAFETY_SEXUALLY_EXPLICIT` | `BLOCK_MEDIUM_AND_ABOVE` | Sexually explicit threshold |
| `VERTEX_SAFETY_DANGEROUS_CONTENT` | `BLOCK_MEDIUM_AND_ABOVE` | Dangerous content threshold |
| `VERTEX_SAFETY_SETTINGS` | (none) | Override all settings as JSON array |

## AWS Credentials

The Go sidecar uses the AWS SDK default credential chain:
- EC2: instance profile (no config needed)
- EKS: IRSA via service account annotation (no config needed)
- Local: `~/.aws/credentials` or `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`

## Moving to EKS

The same containers work as a sidecar pod. Replace `network_mode: host`
with a shared pod network and you're done. The ext_proc sidecar needs:
- A service account with IRSA for Secrets Manager access
- The `LISTEN_ADDR` stays `:9001` (localhost within the pod)
