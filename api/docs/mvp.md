## 🎯 MVP: Distributed Order Processing API

Target MVP:

> User bisa login → melihat produk → membuat order → stok berkurang secara aman → order diproses → payment dibuat → status order berubah.

### Arsitektur MVP

```text
                    ┌──────────────┐
                    │    Client    │
                    └──────┬───────┘
                           │
                          REST
                           │
                           ▼
                ┌─────────────────────┐
                │       Go API        │
                │                     │
                │ Auth                │
                │ Products            │
                │ Orders              │
                │ Payments            │
                └──────┬──────┬───────┘
                       │      │
                ┌──────▼──┐ ┌─▼──────┐
                │Postgres │ │ Redis  │
                └─────────┘ └────────┘
```

**Belum Kafka/gRPC dulu.**

Kenapa?

Karena kita ingin memastikan fundamental-nya benar dulu:

* transaction
* concurrency
* authentication
* idempotency
* error handling
* graceful shutdown
* testing
* Docker

Setelah MVP stabil, baru kita pecah menjadi service/event-driven architecture.

---

# 1. Scope MVP

Kita cuma butuh **5 domain**:

```text
auth
users
products
orders
payments
```

### Endpoint

#### Auth

```http
POST /api/v1/auth/register
POST /api/v1/auth/login
```

#### Products

```http
GET /api/v1/products
GET /api/v1/products/:id
POST /api/v1/products
PATCH /api/v1/products/:id
DELETE /api/v1/products/:id
```

#### Orders

```http
POST /api/v1/orders
GET /api/v1/orders
GET /api/v1/orders/:id
POST /api/v1/orders/:id/cancel
```

#### Payments

```http
POST /api/v1/orders/:id/pay
GET /api/v1/orders/:id/payment
```

Itu **sudah cukup** untuk MVP.

---

# 2. Database

Kita pakai PostgreSQL.

Relasinya kira-kira:

```text
users
  │
  │ 1:N
  ▼
orders
  │
  │ 1:N
  ▼
order_items
  │
  │ N:1
  ▼
products

orders
  │
  │ 1:1
  ▼
payments
```

### users

```text
users
────────────────────
id
email
password_hash
created_at
updated_at
```

### products

```text
products
────────────────────
id
name
description
price
stock
created_at
updated_at
```

### orders

```text
orders
────────────────────
id
user_id
status
total_amount
created_at
updated_at
```

Status:

```text
pending
paid
cancelled
completed
```

### order_items

```text
order_items
────────────────────
id
order_id
product_id
quantity
price
subtotal
```

Perhatikan `price`.

Jangan hanya mengambil harga dari `products` ketika membaca order.

Karena:

```text
Product A
price = 10000

User membeli

Order Item
price = 10000

Besok:

Product A
price = 15000
```

Order lama harus tetap:

```text
10000
```

Ini detail kecil yang bagus untuk menunjukkan pemahaman database/domain modeling.

---

# 3. Payment

```text
payments
────────────────────
id
order_id
amount
status
provider
transaction_id
created_at
updated_at
```

Status:

```text
pending
success
failed
```

Untuk MVP **jangan integrasikan Midtrans/Xendit dulu**.

Kita bikin fake payment provider:

```text
POST /orders/:id/pay
```

misalnya selalu:

```text
success
```

atau bisa dibuat random untuk testing failure.

Nanti ketika arsitektur sudah matang, baru diganti provider sungguhan.

---

# 4. Flow paling penting

Ini inti project-nya.

User:

```http
POST /orders
```

Body:

```json
{
  "items": [
    {
      "product_id": "product-1",
      "quantity": 2
    },
    {
      "product_id": "product-2",
      "quantity": 1
    }
  ]
}
```

Backend:

```text
BEGIN TRANSACTION
        │
        ▼
Lock products
        │
        ▼
Check stock
        │
        ▼
Calculate total
        │
        ▼
Create order
        │
        ▼
Create order_items
        │
        ▼
Decrease stock
        │
        ▼
Create payment
        │
        ▼
COMMIT
```

Kalau salah satu gagal:

```text
ROLLBACK
```

---

# 5. Concurrency adalah selling point pertama

Misalnya:

```text
stock = 1
```

Kemudian:

```text
User A ──────┐
             ├── BUY
User B ──────┘
```

Keduanya request bersamaan.

Kita tidak boleh melakukan:

```sql
SELECT stock FROM products;
```

kemudian baru:

```sql
UPDATE products SET stock = ...
```

secara naif.

Lebih aman menggunakan atomic update:

```sql
UPDATE products
SET stock = stock - $1
WHERE id = $2
  AND stock >= $1;
```

Kemudian cek:

```go
rowsAffected
```

Kalau:

```text
1
```

berarti stok berhasil dikurangi.

Kalau:

```text
0
```

berarti stok tidak cukup.

Ini salah satu bagian yang nanti **wajib kita integration-test**.

---

# 6. Idempotency

Kita juga masukkan sejak MVP.

Contoh:

```http
POST /api/v1/orders
Idempotency-Key: 550e8400-e29b
```

Client timeout.

Client retry:

```http
POST /api/v1/orders
Idempotency-Key: 550e8400-e29b
```

Server harus mengembalikan order yang sama.

Bukan membuat:

```text
Order #1
Order #2
```

Kita bisa punya:

```text
idempotency_keys
────────────────────
key
user_id
request_hash
response
created_at
```

Ini akan menjadi fondasi bagus ketika nanti masuk ke distributed system.

---

# 7. Struktur project Go

Aku akan bikin seperti ini:

```text
order-platform/
│
├── cmd/
│   └── api/
│       └── main.go
│
├── internal/
│   ├── auth/
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── model.go
│   │
│   ├── product/
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── model.go
│   │
│   ├── order/
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── model.go
│   │
│   ├── payment/
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── model.go
│   │
│   └── user/
│       ├── repository.go
│       └── model.go
│
├── internal/platform/
│   ├── database/
│   ├── redis/
│   ├── logger/
│   └── auth/
│
├── migrations/
│   ├── 000001_create_users.sql
│   ├── 000002_create_products.sql
│   ├── 000003_create_orders.sql
│   ├── 000004_create_order_items.sql
│   ├── 000005_create_payments.sql
│   └── 000006_create_idempotency_keys.sql
│
├── tests/
│   └── integration/
│
├── Dockerfile
├── compose.yaml
├── Makefile
├── go.mod
└── README.md
```

---

# 8. Tech stack MVP

Aku pilih:

```text
Go
│
├── net/http
├── PostgreSQL
├── Redis
├── golang-migrate / Goose
├── JWT
├── Docker
└── Testcontainers
```

Untuk HTTP router, kamu bisa pakai **Chi** kalau ingin ergonomi routing/middleware lebih enak, tapi `net/http` juga sangat valid.

Aku justru tidak akan memasukkan terlalu banyak library.

---

# 9. Development roadmap

Kita kerjakan dalam fase.

### Phase 1 — Foundation

```text
[x] Go module
[x] Config
[x] Logger
[x] PostgreSQL
[x] Docker Compose
[x] Migration
[x] Graceful shutdown
[x] /livez
[x] /readyz
```

---

### Phase 2 — Auth

```text
POST /auth/register
POST /auth/login
```

Dengan:

```text
bcrypt
JWT
middleware
```

---

### Phase 3 — Product

```text
POST   /products
GET    /products
GET    /products/:id
PATCH  /products/:id
DELETE /products/:id
```

---

### Phase 4 — Order ⭐

Ini bagian terpenting.

```text
POST /orders
GET /orders
GET /orders/:id
POST /orders/:id/cancel
```

Implement:

```text
transaction
stock reservation
concurrency protection
order snapshots
```

---

### Phase 5 — Payment

```text
POST /orders/:id/pay
GET  /orders/:id/payment
```

Implement:

```text
idempotency
payment state machine
transaction safety
```

---

### Phase 6 — Testing

Minimal:

```text
unit test
integration test
concurrency test
```

Contoh test:

```text
stock = 10

100 concurrent orders
quantity = 1
```

Expected:

```text
10 successful
90 rejected
stock = 0
```

🔥

---

# 10. Setelah MVP selesai

> **Catatan revisi.** Bagian ini awalnya berisi urutan V1 → V5:
> modular monolith → gRPC → Kafka → observability → load testing.
> Urutan itu **dibalik** setelah review, dan roadmap yang berlaku sekarang
> ada di bagian di bawah ini.
>
> Alasannya: memecah service di fase awal berarti memutuskan
> "sistem ini perlu dipecah" tanpa data apa pun yang membuktikannya.
> Belum ada baseline latency, belum ada tracing, belum diketahui
> bottleneck-nya di mana.
>
> Konsekuensi konkretnya: begitu Order dan Inventory jadi dua service
> dengan dua database, transaksi atomik yang jadi selling point MVP ini —
> `FOR UPDATE` + atomic decrement dalam satu `BEGIN` — hilang, dan
> digantikan saga serta compensating transaction. Itu harga yang mahal
> untuk dibayar sebelum tahu apakah memang dibutuhkan.
>
> Karena itu observability dan load testing dipindah ke depan:
> keduanya yang mengubah roadmap ini dari daftar keinginan
> menjadi keputusan berbasis data.

Roadmap dilacak sebagai milestone dan issue di GitHub.

### Phase 7 — Hardening & Cleanup

Menutup hutang MVP sebelum menambah teknologi baru.

```text
Redis dipakai untuk rate limiting (sebelumnya nganggur)
Fix bug /readyz (WriteHeader berkali-kali)
Cleanup dead code di PlaceOrder
Compose profile untuk menjalankan full stack
```

### Phase 8 — Observability

```text
Go
 │
 └── OpenTelemetry
       │
       ├── Traces → Jaeger          (dikerjakan lebih dulu)
       ├── Logs   → korelasi trace_id
       └── Metrics → Prometheus → Grafana
```

Tracing didahulukan karena yang dibutuhkan sebelum load testing adalah
kemampuan melihat **di mana** waktu request habis — dan itu tracing,
bukan metrics. Metrics menyusul sebagai issue terpisah supaya tidak
memasang banyak tool asing sekaligus.

### Phase 9 — Load Testing & Baseline

```text
k6
 │
 ▼
Skenario: baca produk, order tersebar, order rebutan satu produk
 │
 ▼
Ukur: P50, P95, P99, RPS, error rate
```

Dijalankan **lokal** lewat Docker Compose, bukan di VPS — compute VPS
menambah noise sehingga yang terukur jadi infrastruktur, bukan kode.

Fase ini menghasilkan data yang menentukan isi Phase 10-12.

### Phase 10 — Modular Monolith

```text
                Go API
                   │
        ┌──────────┼──────────┐
        ▼          ▼          ▼
      Auth       Order      Product
        │          │          │
        └──────────┼──────────┘
                   ▼
               PostgreSQL
```

Batas antar modul dipertegas, tapi tetap satu proses dan satu database.
Murah, reversible, dan mempermudah pemecahan nanti bila terbukti perlu.

### Phase 11 — Event-Driven

```text
Order Service
      │
      ▼
  outbox table  (ditulis dalam transaksi yang sama)
      │
      ▼
    Kafka
      │
      ▼
 Consumer non-kritis (audit / notification)
```

Outbox pattern dipakai karena menulis ke database dan mem-publish ke
Kafka tidak bisa dilakukan secara atomik. Consumer pertama sengaja
non-kritis — memindahkan pengurangan stok ke event berarti melepas
jaminan atomik, dan itu bukan tempat untuk belajar Kafka.

### Phase 12 — Service Split (Decision)

```text
Order Service
      │
      │ gRPC
      ▼
Inventory Service
```

**Belum tentu dikerjakan.** Keputusannya diambil dari data Phase 9,
bukan dari asumsi. Kalau alasannya murni ingin belajar gRPC, itu sah
untuk project portfolio — asalkan disadari dan ditulis terbuka.

Apapun keputusannya, alasannya didokumentasikan. Kemampuan menjelaskan
**kenapa tidak memecah service** sering lebih berkesan daripada
memecahnya tanpa alasan.

### Phase 13 — Deployment

```text
VPS
 │
 ├── Docker Compose (profile full)
 ├── Reverse proxy + TLS
 └── Secret di luar repo
```
