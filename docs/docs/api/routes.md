---
title: Route Endpoints
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-wiring.png" alt="Mole managing routes" style={{maxWidth: '180px'}} />
</div>

# Route Endpoints

Route management and advertising.

## POST /routes/advertise

Trigger immediate route advertisement.

**Request:** Empty body

**Response:**
```json
{
  "status": "triggered",
  "message": "route advertisement triggered"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/routes/advertise
```

## Use Cases

Trigger immediate advertisement after:
- Configuration changes
- Adding new exit routes
- Network topology changes

Normally routes are advertised periodically based on `routing.advertise_interval` (default 2 minutes).
