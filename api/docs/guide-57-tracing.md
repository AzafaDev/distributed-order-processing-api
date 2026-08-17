# Guide: Issue #57 — OpenTelemetry Tracing + Jaeger

Guide ini tidak berisi solusi lengkap. Isinya: urutan kerja, signature yang kamu butuh,
jebakan yang akan kamu temui, dan checkpoint untuk memverifikasi tiap langkah.
Kalau macet lebih dari ~20 menit di satu step, baru tanya.

**Keputusan yang sudah diambil:**
- Pakai `otelhttp` manual (bukan `otelchi`) — tujuannya supaya kamu paham masalah urutan middleware.
- `/sleep` di-skip. Yang di-exclude hanya `/livez` dan `/readyz`.
- Scope termasuk bonus DB + Redis span (Step 7).

---

## Step 0 — Pahami dulu, sebelum nulis kode

Jawab empat pertanyaan ini untuk dirimu sendiri (di issue juga ada). Kalau belum bisa jawab,
baca https://opentelemetry.io/docs/concepts/signals/traces/ dulu.

1. Kenapa parent-child span dibawa lewat `context.Context`, bukan variabel global?
   (Petunjuk: apa yang terjadi kalau 50 request jalan bersamaan di 50 goroutine?)
2. Span disimpan di buffer sebelum dikirim. Apa yang hilang kalau proses mati tanpa shutdown?
3. Kenapa `/api/orders/{id}` dipakai sebagai atribut, bukan `/api/orders/abc-123`?
   (Petunjuk: bayangkan 10.000 order berbeda. Berapa banyak nama operasi unik yang tercipta?)

Yang nomor 3 akan langsung kamu rasakan konsekuensinya di Step 5.

---

## Step 1 — Jaeger di Docker Compose

Tambah service di `compose.yaml`. Yang perlu kamu tentukan sendiri:

- Image: `jaegertracing/all-in-one` (pin versinya, jangan `latest`)
- Port yang di-expose: **16686** (UI), **4318** (OTLP/HTTP), **4317** (OTLP/gRPC)
- Profile: samakan dengan `server`, yaitu `["full"]`

Lalu tambahkan `jaeger` ke `depends_on` milik `server`, dan set env `OTEL_EXPORTER_OTLP_ENDPOINT`
di service `server` yang menunjuk ke hostname `jaeger` (bukan `localhost` — kenapa?).

**Checkpoint:** `docker compose --profile full up jaeger`, lalu buka http://localhost:16686.
UI harus muncul walau belum ada trace apa pun.

**Pertanyaan:** kenapa Jaeger butuh dua port OTLP (4317 dan 4318)? Kamu nanti pilih salah satu di Step 3.

---

## Step 2 — Config

Di `internal/platform/config/config.go`, tambah field:

| Field | Env | Default |
|---|---|---|
| `OtelExporterEndpoint` | `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4318` |
| `OtelServiceName` | `OTEL_SERVICE_NAME` | `order-api` |
| `OtelSampleRatio` | `OTEL_SAMPLE_RATIO` | `1.0` |

Helper `getEnv` sudah ada. Untuk `OtelSampleRatio` kamu butuh helper baru bertipe `float64` —
ikuti pola `getEnvInt` yang sudah ada di file itu.

Update juga `.env.example`.

**Kenapa sample ratio?** Di Phase 9 kamu akan menembak ribuan request. Jaeger all-in-one
menyimpan trace **di memori**. Sampling 100% saat load test = OOM. Default 1.0 untuk development,
turunkan lewat env saat load test.

---

## Step 3 — Package `internal/platform/tracing`

Buat file baru `internal/platform/tracing/tracing.go`. Target signature:

```go
func New(ctx context.Context, cfg *config.Config) (*sdktrace.TracerProvider, error)
```

Dependency yang perlu di-`go get`:

```
go.opentelemetry.io/otel
go.opentelemetry.io/otel/sdk
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
```

Catatan: sebagian sudah ada di `go.mod` tapi statusnya `// indirect` (tertarik dari pgx/redis).
Setelah kamu import langsung, `go mod tidy` akan memindahkannya ke blok `require` utama.

Isi fungsi tersebut, urutannya:

1. **Exporter** — `otlptracehttp.New(ctx, ...)`. Perhatikan: endpoint-nya default pakai HTTPS.
   Jaeger lokal tidak punya TLS. Ada satu option yang wajib kamu tambahkan. Cari di godoc `otlptracehttp`.
2. **Resource** — `resource.New(...)` dengan atribut `semconv.ServiceName(...)`.
   Ini yang menentukan AC "service name bukan `unknown_service`". Kalau kamu skip resource,
   itulah persis yang muncul di Jaeger.
3. **Sampler** — `sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))`.
   Kenapa dibungkus `ParentBased`? Pikirkan: kalau nanti ada service kedua, dan service pertama
   memutuskan "trace ini di-sample", apa yang harus dilakukan service kedua?
4. **TracerProvider** — `sdktrace.NewTracerProvider` dengan `WithBatcher(exporter)`,
   `WithResource(res)`, `WithSampler(sampler)`.
5. **Register global** — `otel.SetTracerProvider(tp)` dan
   `otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))`.

Return `tp` — pemanggil yang bertanggung jawab men-shutdown.

**Pertanyaan:** apa beda `WithBatcher` dan `WithSyncer`? Kenapa untuk production selalu `WithBatcher`,
dan apa konsekuensinya terhadap Step 4?

---

## Step 4 — Shutdown di `cmd/api/main.go`

Panggil `tracing.New` **sebelum** `server.BuildApp` (Step 7 butuh TracerProvider sudah terdaftar
saat pool dan redis client dibuat).

Lalu daftarkan shutdown di blok graceful shutdown yang sudah ada. Urutannya penting:

```
serv.Shutdown()      ← selesai dulu, biar semua span tertutup
tp.Shutdown()        ← baru flush span ke Jaeger
app.DB.Close()
app.Redis.Close()
```

**Kenapa `tp.Shutdown` harus setelah `serv.Shutdown`?** Kalau dibalik, request yang masih
in-flight akan menghasilkan span yang masuk ke provider yang sudah mati — span-nya hilang diam-diam.

`tp.Shutdown(ctx)` mengembalikan `error` — tangani seperti `serv.Shutdown`, jangan di-ignore.
Pakai `shutdownCtx` yang sudah ada.

**Checkpoint:** jalankan app, kirim satu request, langsung `Ctrl+C`. Span request itu harus tetap
muncul di Jaeger. Coba juga versi tanpa `tp.Shutdown` — lihat sendiri span-nya hilang.
Itu jawaban pertanyaan #2 di Step 0, dibuktikan sendiri.

---

## Step 5 — Middleware (bagian paling tricky)

### 5a. Pasang otelhttp

Di `internal/server/router.go`:

```go
r.Use(otelhttp.NewMiddleware("http.server"))
```

Jalankan, kirim `GET /api/products`, buka Jaeger.

**Kamu akan lihat masalahnya:** semua trace bernama `http.server`. Semua endpoint terlihat sama.
Tidak ada route pattern di mana pun. Berhenti sejenak dan pikirkan **kenapa**.

Petunjuk: `r.Use` mendaftarkan middleware yang jalan **sebelum** chi melakukan routing.
Pada saat `otelhttp` membuat span, chi belum tahu request ini cocok ke route yang mana.
Ini bukan bug otelhttp — ini konsekuensi arsitektural dari "middleware membungkus router".

### 5b. Perbaiki dengan middleware kedua

Bikin middleware sendiri, taruh **setelah** `otelhttp` di rantai `r.Use` (jadi ia berada di dalam).
Logikanya:

```go
func routeTag(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        next.ServeHTTP(w, r)          // ← biarkan chi routing dulu
        // baru di sini pattern-nya tersedia
    })
}
```

Setelah `next.ServeHTTP` selesai:
- Ambil pattern: `chi.RouteContext(r.Context()).RoutePattern()`
- Ambil span aktif: `trace.SpanFromContext(r.Context())`
- Set nama span jadi `"GET /api/orders/{id}"` dan tambah atribut `semconv.HTTPRoute(pattern)`

**Kenapa ini bekerja:** chi mengisi `RouteContext` secara in-place di request yang sama.
Setelah handler selesai, pattern-nya sudah terisi, dan span-nya masih hidup karena
`otelhttp` berada di lapisan luar dan belum meng-`End()`.

Jaga terhadap pattern kosong (404) — jangan set nama span jadi string kosong.

### 5c. Exclude health check

Pakai `otelhttp.WithFilter(...)` sebagai option di `NewMiddleware`. Filter menerima `*http.Request`
dan mengembalikan `bool` — **`false` berarti request tidak di-trace**. Cek `r.URL.Path` terhadap
`/livez` dan `/readyz`.

**Checkpoint akhir Step 5:**
- `GET /api/products` → trace bernama `GET /api/products`, punya atribut `http.route`, `http.method`, `http.status_code`
- `GET /api/orders/{id}` dengan id berbeda-beda → semuanya masuk ke **satu** nama operasi yang sama di Jaeger
- `GET /livez` dipanggil 20x → nol trace baru di Jaeger

---

## Step 6 — Verifikasi AC

Jalankan `docker compose --profile full up --build`, lalu:

- [ ] Jaeger UI terbuka di :16686
- [ ] `POST /api/orders` muncul sebagai satu trace
- [ ] Span punya atribut method, route, status code
- [ ] Service name = `order-api`, bukan `unknown_service:api`
- [ ] SIGTERM tepat setelah request → span terakhir tetap sampai
- [ ] `/livez` dan `/readyz` tidak ter-trace
- [ ] Endpoint OTLP dibaca dari config

---

## Step 7 — Bonus: span per-layer (ini yang bikin tracing berguna)

Sampai Step 6, trace kamu cuma bilang *"request ini 400ms"*. Tidak ada petunjuk 400ms itu habis
di mana. Padahal itu justru alasan issue ini ada. Step 7 memecahnya.

### 7a. PostgreSQL

`go get github.com/exaring/otelpgx`

Di `internal/platform/database/database.go`, setelah `pgxpool.ParseConfig`, sebelum
`pgxpool.NewWithConfig`, pasang tracer ke `cfgPool.ConnConfig.Tracer`.
Baca README otelpgx untuk nama konstruktornya.

### 7b. Redis

`go get github.com/redis/go-redis/extra/redisotel/v9`

Di `internal/server/app.go`, setelah `redis.NewClient(...)`, panggil `redisotel.InstrumentTracing(rdb)`.
Fungsi ini mengembalikan `error` — tangani.

### Checkpoint

Kirim `POST /api/orders` lagi. Sekarang satu trace harus berisi pohon:

```
POST /api/orders                      120ms
├─ SET idempotency:...                  2ms   (redis)
├─ BEGIN                                1ms
├─ SELECT products WHERE id = $1        8ms
├─ INSERT INTO orders ...              15ms
└─ COMMIT                               4ms
```

Inilah bentuk yang kamu butuh di Phase 9. "P99 = 400ms" jadi "P99 = 400ms, 340ms-nya di
`SELECT products`, dan itu terjadi karena tidak ada index".

**Yang perlu kamu perhatikan:** span DB hanya muncul kalau `ctx` diteruskan sampai ke query.
Kalau ada tempat di repository yang pakai `context.Background()` alih-alih ctx dari request,
span-nya akan yatim — muncul sebagai trace terpisah, bukan child. Kalau kamu lihat gejala itu
di Jaeger, itu bug propagasi context yang selama ini tidak kelihatan. Tracing baru saja
menemukan bug untukmu.

---

## Yang sengaja tidak masuk scope

- **Metrics** (RED: rate/error/duration) — sinyal berbeda, issue terpisah.
- **Log correlation** (inject trace_id ke slog) — sangat berguna, tapi bikin issue sendiri.
- **Instrumentasi manual di service layer** (`tracer.Start(ctx, "OrderService.Create")`) —
  tunggu sampai Step 7 menunjukkan ada gap durasi yang tidak terjelaskan oleh span DB/Redis.
  Instrumentasi manual sebelum ada pertanyaan konkret = noise.
