# Govershop Backend API

Golang backend service for Govershop functionality, handling product catalog, orders, payments, and integrations with Digiflazz & Pakasir.

## 🚀 Deployment

The backend is currently deployed on **Easypanel**.

- **Production URL**: `https://govershop-govershop-be.lzfki7.easypanel.host`
- **Documentation (Swagger)**: *See `docs/swagger.yaml`*

### Deploying with Docker
The project includes a `Dockerfile` optimized for production (alpine based).

```bash
docker build -t govershop-backend .
docker run -p 8080:8080 --env-file .env govershop-backend
```

### Environment Variables
Ensure these variables are set in your deployment environment:

| Variable | Description |
|----------|-------------|
| `PORT` | Application port (default: 8080) |
| `ENV` | Environment (`production` / `development`) |
| `DATABASE_URL` | PostgreSQL connection string |
| `JWT_SECRET` | Secret key for JWT signing |
| `ADMIN_PASSWORD` | Password for admin authentication |
| `DIGIFLAZZ_USERNAME` | Digiflazz username |
| `DIGIFLAZZ_API_KEY` | Digiflazz production/dev key |
| `DIGIFLAZZ_WEBHOOK_SECRET` | Secret for verifying Digiflazz webhooks |
| `PAKASIR_API_KEY` | Pakasir API Key |

---

## 🛠️ Local Development

### Prerequisites
- Go 1.22+
- PostgreSQL

### Installation
1. Clone repository
2. Copy `.env.example` to `.env` (or create one)
3. Run:
   ```bash
   go mod download
   go run main.go
   ```

### API Endpoints Overview

#### Public
- `GET /api/v1/products` - List products
- `GET /api/v1/products/{sku}` - Product details
- `POST /api/v1/validate-account` - Validate game ID
- `POST /api/v1/calculate-price` - Calculate total price with fees

#### Orders
- `POST /api/v1/orders` - Create order
- `POST /api/v1/orders/{id}/pay` - Initiate payment (QRIS/VA)
- `GET /api/v1/orders/{id}/status` - Check status

#### Admin (Protected)
- `GET /api/v1/admin/dashboard` - Stats
- `POST /api/v1/admin/topup/custom` - Custom topup (admin only)

---

## 🔐 Security
- **Admin**: Uses JWT Authentication + TOTP (2FA) for sensitive actions like manual topup.
- **Webhooks**: Signature verification enabled for Digiflazz & Pakasir webhooks.
