# NATS streaming with gogoproto

# Usage

Build image and run docker compose.

```bash
make queue
docker-compose up
```

Send job

```
make send_job 
```

 * Prometheus: localhost:9090
 * Jaeger: localhost:16686
 * Liveness: localhost:8081/live
 * Readiness: localhost:8081/ready

# Test coverage

```bash
make cover
```

