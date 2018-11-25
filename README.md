# NATS streaming with gogoproto

# Usage

Build image and run docker compose.

```bash
make queue
docker-compose up
```

Send job to nats

```
make send_job 
```

Send job to rabbit

```
go run cmd/rabbit_client/main.go
```

 * Prometheus: localhost:9090
 * Jaeger: localhost:16686

# Test coverage

```bash
make cover
```

