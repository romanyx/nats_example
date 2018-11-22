# NATS gogoproto request/reply and streaming

# Setup

Run nats stream server.

```bash
docker run -p 4223:4223 -p 8223:8223 nats-streaming -p 4223 -m 8223 -ns nats://demo.nats.io:4222 -cid=test-cluster
```
