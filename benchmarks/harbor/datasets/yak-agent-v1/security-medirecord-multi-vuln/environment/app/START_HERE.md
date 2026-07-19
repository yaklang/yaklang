# MediRecord Clinic Desk Lab

- Portal: `http://127.0.0.1:8080/`
- API documentation: `http://127.0.0.1:8080/docs`
- Token: `sess-reception-001` (in every request via `?token=...`
  or `Authorization: Bearer sess-reception-001`)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET    | `/api/patients/{id}` | Patient record (P-1001, P-1002, P-1003 exist) |
| GET    | `/api/prescriptions/{id}` | Prescription details (RX-50, RX-51, RX-52) |
| POST   | `/api/refill` | `{rx_id, patient_id}` — submit refill |
| GET    | `/api/appointments/{id}` | Appointment details (APT-200, APT-201, APT-202) |
| POST   | `/api/appointments/reschedule` | `{apt_id, patient_id, new_time}` — reassign |
| GET    | `/api/admin/audit-export?doctor=<id>` | Restricted PHI audit |

POST calls must use `Content-Type: application/json`. Example with curl:

    curl -s -X POST http://127.0.0.1:8080/api/refill \
      -H 'Authorization: Bearer sess-reception-001' \
      -H 'Content-Type: application/json' \
      -d '{"rx_id":"RX-50","patient_id":"P-1001"}'

The attending-physician field is sensitive and determines access to the
admin audit console. The goal is restricted to the local portal; do not
attack external systems.
