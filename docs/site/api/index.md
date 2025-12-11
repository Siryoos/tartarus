# API Reference

The Olympus API provides RESTful endpoints for managing sandboxes.

## Base URL

```
http://localhost:8080/api/v1
```

## Authentication

=== "Bearer Token"

    ```bash
    curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/sandboxes
    ```

=== "API Key"

    ```bash
    curl -H "X-API-Key: $API_KEY" http://localhost:8080/api/v1/sandboxes
    ```

=== "mTLS"

    ```bash
    curl --cert client.crt --key client.key --cacert ca.crt https://olympus/api/v1/sandboxes
    ```

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/sandboxes` | Create a sandbox |
| GET | `/sandboxes` | List sandboxes |
| GET | `/sandboxes/{id}` | Get sandbox details |
| DELETE | `/sandboxes/{id}` | Kill a sandbox |
| POST | `/sandboxes/{id}/exec` | Execute command |
| GET | `/sandboxes/{id}/logs` | Get logs |

## Common Responses

| Code | Description |
|------|-------------|
| 200 | Success |
| 201 | Created |
| 400 | Bad Request |
| 401 | Unauthorized |
| 404 | Not Found |
| 500 | Internal Error |

## Next

- [Sandbox API](sandbox.md)
- [Template API](template.md)
