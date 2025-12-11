# Sandbox API

## Create Sandbox

```http
POST /api/v1/sandboxes
```

### Request

```json
{
  "template": "python-ds",
  "name": "my-sandbox",
  "resources": {
    "cpu": 2,
    "memory": 4096
  },
  "env": {
    "DEBUG": "true"
  },
  "timeout": "1h"
}
```

### Response

```json
{
  "id": "sbx-abc123",
  "name": "my-sandbox",
  "status": "PENDING",
  "createdAt": "2024-01-15T10:00:00Z"
}
```

---

## List Sandboxes

```http
GET /api/v1/sandboxes
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `status` | string | Filter by status |
| `tenant` | string | Filter by tenant |
| `limit` | int | Max results |

### Response

```json
{
  "sandboxes": [
    {
      "id": "sbx-abc123",
      "name": "my-sandbox",
      "status": "RUNNING",
      "template": "python-ds"
    }
  ]
}
```

---

## Get Sandbox

```http
GET /api/v1/sandboxes/{id}
```

### Response

```json
{
  "id": "sbx-abc123",
  "name": "my-sandbox",
  "status": "RUNNING",
  "template": "python-ds",
  "resources": {
    "cpu": 2,
    "memory": 4096
  },
  "startedAt": "2024-01-15T10:00:05Z",
  "node": "node-1"
}
```

---

## Kill Sandbox

```http
DELETE /api/v1/sandboxes/{id}
```

### Response

```json
{
  "id": "sbx-abc123",
  "status": "TERMINATED"
}
```

---

## Execute Command

```http
POST /api/v1/sandboxes/{id}/exec
```

### Request

```json
{
  "command": ["python", "-c", "print('hello')"],
  "stdin": "",
  "interactive": false
}
```

### Response

```json
{
  "stdout": "hello\n",
  "stderr": "",
  "exitCode": 0
}
```
