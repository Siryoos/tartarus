# Template API

## List Templates

```http
GET /api/v1/templates
```

### Response

```json
{
  "templates": [
    {
      "name": "python-ds",
      "version": "1.0.0",
      "category": "data-science"
    }
  ]
}
```

---

## Get Template

```http
GET /api/v1/templates/{name}
```

### Response

```json
{
  "name": "python-ds",
  "version": "1.0.0",
  "author": "Tartarus Team",
  "description": "Python Data Science environment",
  "spec": {
    "baseImage": "python:3.9-slim",
    "packages": ["numpy", "pandas"],
    "resources": {
      "cpu": 2,
      "memory": 4096
    }
  }
}
```

---

## Create Template

```http
POST /api/v1/templates
```

### Request

```json
{
  "name": "my-template",
  "version": "1.0.0",
  "spec": {
    "baseImage": "node:20-slim",
    "packages": ["express"],
    "resources": {
      "cpu": 1,
      "memory": 1024
    }
  }
}
```

---

## Delete Template

```http
DELETE /api/v1/templates/{name}
```
