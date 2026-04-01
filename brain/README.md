# Brain FastAPI Base

Base backend FastAPI untuk "otak" AI Agent.
WA bot (Go) berperan sebagai kurir pesan ke backend ini.

## Struktur Clean Architecture (sederhana)

- `backend/domain`:
  - Entitas domain murni (`HealthStatus`).
- `backend/application`:
  - Use case aplikasi (`CheckHealthUseCase`).
- `backend/interface/api`:
  - HTTP adapter (router FastAPI, response schema).
- `backend/bootstrap`:
  - Komposisi dependency dan pembuatan aplikasi FastAPI.
- `app.py`:
  - Entrypoint aplikasi.

## Endpoint Contoh

- `GET /api/v1/checkhealth`

Contoh response:

```json
{
  "status": "ok",
  "service": "brain-agent-api",
  "timestamp": "2026-04-01T12:34:56.789012+00:00"
}
```

## Jalankan Lokal

```bash
pip install -r requirements.txt
uvicorn app:app --reload --host 0.0.0.0 --port 8000
```
