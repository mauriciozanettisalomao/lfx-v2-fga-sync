# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Service Overview

FGA Sync is a high-performance Go microservice that synchronizes authorization data between NATS messaging and OpenFGA (Fine-Grained Authorization). It provides cached relationship checks and real-time access control updates for the LFX Platform v2.

## Architecture

### Core Components

- **NATS Message Handlers**: Process access checks, project updates, and project deletions
- **OpenFGA Client**: Manages authorization relationships and batch operations
- **JetStream Cache**: High-performance KeyValue store for relationship caching
- **Health Endpoints**: Kubernetes-ready liveness and readiness probes

### Message Flow

1. NATS messages arrive on environment-prefixed subjects (e.g., `dev.lfx.access_check.request`)
2. Queue groups ensure load balancing across service instances
3. Handlers process messages, interact with cache/OpenFGA, and send replies
4. Cache invalidation occurs on project updates/deletions

### Key Dependencies

- `github.com/nats-io/nats.go` - NATS messaging client
- `github.com/openfga/go-sdk` - OpenFGA authorization client
- Standard library for HTTP server and JSON processing

## Common Development Commands

```bash
# Build and test
make build          # Build the fga-sync binary
make test           # Run all tests
make test-coverage  # Run tests with coverage report
make check          # Format, vet, lint, and security scan

# Development
make dev           # Build with debug symbols and race detection
make run           # Build and run the service locally
make clean         # Clean build artifacts

# Docker operations
make docker-build  # Build Docker image
make docker-run    # Run service in Docker container

# Code quality
make fmt           # Format Go code
make lint          # Run golangci-lint
make vet           # Run go vet
make gosec         # Run security scanner
```

## Configuration

### Required Environment Variables

- `NATS_URL`: NATS server connection URL (e.g., `nats://nats:4222`)
- `FGA_API_URL`: OpenFGA API endpoint (e.g., `http://openfga:8080`)

### Optional Environment Variables

- `CACHE_BUCKET`: JetStream KeyValue bucket name (default: `fga-sync-cache`)
- `LFX_ENVIRONMENT`: Environment prefix for NATS subjects (default: `dev`)
- `PORT`: HTTP server port (default: `8080`)
- `DEBUG`: Enable debug logging (default: `false`)

## Message Formats

### Access Check Request (`{env}.lfx.access_check.request`)

```
project:123#admin@user:456
project:789#viewer@user:456
```

Multiple relationship checks, one per line. Format: `object#relation@user`

### Project Update Message (`{env}.lfx.update_access.project`)

```json
{
  "uid": "project-123",
  "public": true,
  "parent_uid": "parent-project-456", 
  "writers": ["user1", "user2"],
  "auditors": ["auditor1"]
}
```

### Project Delete Message (`{env}.lfx.delete_all_access.project`)

```
project-123
```

Simple project UID string.

## Testing

### Running Tests

```bash
# Run all tests
go test ./...

# Run specific test
go test -v ./... -run TestAccessCheckHandler

# Run benchmarks
go test -bench=. ./...

# Integration tests (requires Docker)
./test_integration.sh
```

### Test Structure

- `*_test.go` files contain unit tests for each handler
- `test_integration.sh` runs full service integration tests
- `docker-compose.test.yml` provides test environment with NATS and OpenFGA

## Code Architecture

### Handler Pattern

Each message type has a dedicated handler function:

- `accessCheckHandler()` - Processes authorization queries with caching
- `projectUpdateHandler()` - Manages project permission synchronization  
- `projectDeleteHandler()` - Handles cleanup of project permissions

### Cache Strategy

- **Cache Keys**: Base32-encoded relation tuples (e.g., `rel.{encoded-relation}`)
- **Cache Values**: JSON with `allowed` boolean and `created_at` timestamp
- **Invalidation**: Timestamp-based with configurable staleness tolerance
- **Fallback**: Direct OpenFGA queries on cache miss

### Error Handling

- Structured logging with context
- Graceful degradation (cache miss → OpenFGA query)
- Message reply with error details for debugging
- Service continues running on individual message failures

## Performance Considerations

### Optimization Patterns

- Preallocated slices to reduce garbage collection
- Batch OpenFGA operations (up to 100 tuples per request)
- Cache-first approach with sub-millisecond response times
- Efficient string parsing using `bytes.Cut`

### Monitoring

- Expvar metrics at `/debug/vars` (cache hits/misses/stale hits)
- Structured JSON logging for observability
- Health endpoints for Kubernetes probes

## Deployment

### Local Development

```bash
# Set environment variables
export NATS_URL="nats://localhost:4222"
export FGA_API_URL="http://localhost:8080"
export LFX_ENVIRONMENT="dev"

# Run the service
make run
```

### Kubernetes

```bash
# Deploy with Helm
helm install fga-sync ./charts/lfx-v2-fga-sync \
  --set nats.url=nats://lfx-platform-nats.lfx.svc.cluster.local:4222 \
  --set fga.apiUrl=http://openfga.lfx.svc.cluster.local:8080
```

## Troubleshooting

### Common Issues

- **Build failures**: Ensure Go 1.24+ and run `go mod tidy`
- **NATS connection**: Verify NATS_URL and network connectivity
- **OpenFGA errors**: Check FGA_API_URL and ensure OpenFGA is healthy
- **Cache issues**: Monitor cache hit rates via `/debug/vars`

### Debugging

- Set `DEBUG=true` for verbose logging
- Check service health at `/livez` and `/readyz`
- Monitor Docker container logs for connection issues
- Use `make check` to validate code quality before deployment
