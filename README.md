# Soft-Proxy: Multiplexer Server

Soft-Proxy adalah layanan **Multiplexer Server** (port-sharing) cerdas berbasis bahasa Go yang didesain untuk menyatukan berbagai protokol proxy (VLESS, VMess, Trojan, Reality) ke dalam port publik tunggal (Port 80 untuk HTTP, Port 443 untuk HTTPS/TLS).

> [!WARNING]
> Proyek ini bersifat **eksperimental**. Penggunaan dalam lingkungan produksi harus dilakukan dengan hati-hati karena sangat mungkin terdapat kesalahan konfigurasi (miss), ketidakcocokan sistem, atau malafungsi lainnya.

---

## 📂 Struktur Proyek (Project Structure)

Berikut adalah struktur folder dan berkas proyek Soft-Proxy beserta penjelasannya:

```text
soft/
├── cmd/
│   └── soft-proxy/            # Kode utama layanan server multiplexer
│       └── main.go
├── internal/                  # Package internal pendukung (Reusable modules)
│   ├── acme/                  # Integrasi tantangan ACME Let's Encrypt (HTTP & Cloudflare DNS)
│   │   └── acme.go
│   ├── autoblocker/           # Modul auto-blocker untuk mengamankan port dari scanner/probing
│   │   └── autoblocker.go
│   ├── config/                # Modul pemuatan & hot-reload otomatis berkas config.yaml
│   │   └── config.go
│   ├── core/                  # Engine utama multiplexing TLS, HTTP, WebSocket & sniffing SNI
│   │   ├── conn.go
│   │   ├── proxy.go
│   │   └── server.go
│   └── logger/                # Logging JSON & rotasi otomatis ukuran berkas log
│       └── logger.go
├── config.yaml                # Berkas konfigurasi utama untuk backends & domain
├── go.mod                     # Go Modules file dependensi
├── go.sum                     # Checksum dependensi Go
└── .gitignore                 # Daftar berkas yang diabaikan oleh Git (certs, log)
```

---

## 🏗️ 1. Diagram Arsitektur Multiplexing

Aliran penanganan koneksi masuk oleh `soft-proxy` divisualisasikan melalui diagram berikut:

```mermaid
flowchart TD
    Client["📱 Klien VPN (Android/PC)"]
    SP["🔒 soft-proxy\n(Port 80 & 443)"]
    SNI{"🔍 SNI Sniffing"}
    ACME["✅ TLS Termination\n(Let's Encrypt)"]
    SS["🔐 TLS Termination\n(Self-Signed)"]
    BYPASS["⚡ TLS Bypass\n(Raw Stream)"]
    DETECT{"🧬 Protocol Detection\n(Peek 256 bytes)"}
    NGINX["🌐 Nginx\n(127.0.0.1:8080)"]
    VLESS_TCP["VLESS TCP\n(127.0.0.1:1234)"]
    VMESS_TCP["VMess TCP\n(127.0.0.1:1334)"]
    TROJAN_TCP["Trojan TCP\n(127.0.0.1:1434)"]
    
    REALITY_TCP["VLESS TCP Reality\n(127.0.0.1:10444)"]
    REALITY_XHTTP["VLESS XHTTP Reality\n(127.0.0.1:10445)"]
    REALITY_TCP_NOFLOW["VLESS TCP Reality Flow None\n(127.0.0.1:10446)"]
    
    Proxy_WS["WS Proxy\n(/vless-ws /vmess-ws /trojan-ws)"]
    Proxy_HTTPUpgrade["HTTPUpgrade Proxy\n(/vless-httpup /vmess-httpup /trojan-httpup)"]
    Proxy_gRPC["gRPC Proxy\n(vless-grpc vmess-grpc trojan-grpc)"]
    Proxy_XHTTP["XHTTP Proxy\n(/vless-xhttp /vmess-xhttp /trojan-xhttp)"]

    Client -->|"Port 443"| SP
    Client -->|"Port 80"| SP
    SP --> SNI

    SNI -->|"SNI = yourdomain.com"| ACME
    SNI -->|"SNI = yahoo.com / www.yahoo.com / www.google.com"| BYPASS
    SNI -->|"SNI = domain bebas lainnya"| SS

    ACME --> DETECT
    SS --> DETECT

    DETECT -->|"Byte[0] == 0x00"| VLESS_TCP
    DETECT -->|"56-char hex + CRLF"| TROJAN_TCP
    DETECT -->|"GET/POST/PRI/..."| NGINX
    DETECT -->|"Lainnya (VMess)"| VMESS_TCP

    BYPASS -->|"SNI: yahoo.com"| REALITY_TCP
    BYPASS -->|"SNI: www.google.com"| REALITY_XHTTP
    BYPASS -->|"SNI: www.yahoo.com"| REALITY_TCP_NOFLOW

    NGINX --> Proxy_WS
    NGINX --> Proxy_HTTPUpgrade
    NGINX --> Proxy_gRPC
    NGINX --> Proxy_XHTTP
```

---

## 🚦 2. Alur Keputusan Port & SNI

### Penanganan Port 443 (HTTPS/TLS)
1.  **TLS Bypass (Reality):** SNI dicocokkan dengan domain Reality (seperti `yahoo.com`, `www.google.com`, `www.yahoo.com`). Sambungan byte mentah langsung dialihkan (*piping*) ke port Xray Reality tujuan tanpa membongkar TLS handshake di tingkat multiplexer.
2.  **TLS Termination (Standar):** Jika SNI adalah domain resmi (`yourdomain.com`) atau domain bebas lainnya, jabat tangan TLS diselesaikan di multiplexer menggunakan sertifikat resmi (ACME Let's Encrypt) atau fallback self-signed.
3.  **Protocol Sniffing:** Setelah TLS didekripsi, sistem membaca 256 byte data awal (*peek decrypted stream*) untuk mengenali protokol internal:
    *   `Byte[0] == 0x00` ➡️ VLESS TCP (`127.0.0.1:1234`)
    *   `56-character Hexadecimal + CRLF` ➡️ Trojan TCP (`127.0.0.1:1434`)
    *   `GET/POST/HTTP/..."` ➡️ Nginx (`127.0.0.1:8080`) untuk melayani WebSocket/gRPC/HTTPUpgrade path.
    *   Lainnya ➡️ VMess TCP (`127.0.0.1:1334`)

### Penanganan Port 80 (HTTP)
*   Jika berisi HTTP/2 Preface (`PRI `) atau path proxy khusus (`/vless-`, `/vmess-`, `/trojan-`), koneksi langsung dialihkan ke Nginx (`127.0.0.1:8080`).
*   Jika bukan permintaan proxy (misal `.well-known/acme-challenge/`), diproses oleh HTTP server bawaan Go untuk tantangan sertifikat Let's Encrypt atau langsung dialihkan (301 redirect) ke port HTTPS (443).

---

## ✨ Fitur Utama (Core Features)

Layanan `soft-proxy` dilengkapi dengan fitur-fitur canggih untuk menjamin performa, keamanan, dan fungsionalitas tingkat tinggi:

1. **Multiplexing Berbasis Protokol & SNI (SNI & Protocol-Based Multiplexing)**
   * Mencegat lalu lintas data pada Port 80 & 443, menyaring domain SNI lewat `server.go`.
   * Melakukan dekripsi TLS standar dan *peeking stream* (256 byte pertama via `conn.go`) untuk membedakan secara instan protokol VLESS, Trojan, VMess, atau HTTP (Nginx).

2. **TLS Bypass Mentah (Zero-Decryption TLS Bypass / Reality Support)**
   * Mendukung pengalihan lalu lintas Xray Reality secara langsung (*raw TCP stream piping* via `proxy.go`) ke backend port Reality tanpa mendekripsi TLS, menghemat penggunaan CPU dan mempertahankan efektivitas obfuscation XTLS-Reality.

3. **Perlindungan Terhadap Pemindaian Port (Auto-Blocker / Active Probing Protection)**
   * Memanfaatkan modul `autoblocker.go` untuk mendeteksi scanner atau koneksi ilegal (seperti handshake TLS parsial/gagal).
   * Memblokir IP mencurigakan secara dinamis (*temporary blacklist*) dengan efisien menggunakan goroutine sweeper terpusat tanpa membebani memori server.

4. **Manajemen Sertifikat ACME Terintegrasi (Automatic ACME DNS API & Multi-Domain Support)**
   * **ACME DNS API (DNS-01 Challenge):** Memanfaatkan Cloudflare DNS API Token untuk menyelesaikan tantangan DNS-01. Mekanisme ini meniadakan kebutuhan eksposur Port 80 untuk proses verifikasi Let's Encrypt.
   * **Wildcard Certs Support:** Melalui DNS-01 API challenge, `soft-proxy` secara sah mendukung penerbitan dan pembaruan sertifikat wildcard (`*.domain.com` disimpan sebagai `_wildcard.domain.com.crt`) secara otomatis.
   * **Single & Multi-Domain (Multi-Tenancy):** Mengelola sertifikat terpisah untuk berbagai domain berbeda secara simultan. Sertifikat dipilih secara dinamis berdasarkan SNI klien (SNI-based lookup) saat handshake TLS berlangsung.
   * **Fallback Self-Signed:** Otomatis membuat sertifikat mandiri (*self-signed certificate fallback*) untuk domain tidak terdaftar agar TLS Handshake tetap berjalan stabil sebelum sertifikat resmi terbit.

5. **Pemuatan Ulang Konfigurasi Tanpa Henti (Thread-Safe Hot-Reload)**
   * Memantau berkas `config.yaml` secara berkala via `config.go`. Perubahan backend atau domain Reality akan dimuat secara dinamis tanpa perlu mematikan atau menghentikan koneksi aktif di server multiplexer.

6. **Pencatatan Aktivitas Terstruktur & Rotasi Log (Structured JSON Logging)**
   * Menggunakan logger terpusat di `logger.go` yang mencatat log dalam format JSON.
   * Mencegah kepenuhan penyimpanan disk melalui sistem *log rotation* otomatis berdasarkan ukuran berkas.

---

## 🗺️ 3. Pemetaan Port Lengkap

### Port Publik (Eksternal)

| Port | Protokol | Fungsi |
|------|----------|--------|
| **80** | HTTP | Tantangan Let's Encrypt (ACME), HTTP Redirection ke 443, dan Plain WebSocket/gRPC bypass |
| **443** | HTTPS/TLS | TLS Bypass (Reality) & TLS Termination (ACME / Self-Signed) |

### Port Lokal Xray & Nginx (127.0.0.1)

| Port | Protokol | Transport | Keterangan |
|------|----------|-----------|------------|
| **1234** | VLESS | TCP | Untuk koneksi TCP+TLS (setelah didekripsi soft-proxy) |
| **1235** | VLESS | WebSocket | Jalur path: `/vless-ws` |
| **1236** | VLESS | HTTPUpgrade | Jalur path: `/vless-httpupgrade` |
| **1237** | VLESS | gRPC | Nama Service: `vless-grpc` |
| **1238** | VLESS | XHTTP | Jalur path: `/vless-xhttp` |
| **1334** | VMess | TCP | Untuk koneksi TCP+TLS (setelah didekripsi soft-proxy) |
| **1335** | VMess | WebSocket | Jalur path: `/vmess-ws` |
| **1336** | VMess | HTTPUpgrade | Jalur path: `/vmess-httpupgrade` |
| **1337** | VMess | gRPC | Nama Service: `vmess-grpc` |
| **1338** | VMess | XHTTP | Jalur path: `/vmess-xhttp` |
| **1434** | Trojan | TCP | Untuk koneksi TCP+TLS (setelah didekripsi soft-proxy) |
| **1435** | Trojan | WebSocket | Jalur path: `/trojan-ws` |
| **1436** | Trojan | HTTPUpgrade | Jalur path: `/trojan-httpupgrade` |
| **1437** | Trojan | gRPC | Nama Service: `trojan-grpc` |
| **1438** | Trojan | XHTTP | Jalur path: `/trojan-xhttp` |
| **10443** | VLESS | TCP+TLS | Port Fallback TLS Standard (Dengan sertifikat self-signed) |
| **10444** | VLESS | TCP+Reality | XTLS-Reality (SNI: `yahoo.com`, Flow: `xtls-rprx-vision`) |
| **10445** | VLESS | XHTTP+Reality | XTLS-Reality (SNI: `www.google.com`, Flow: `none`, path: `/vless-xhttp-reality`) |
| **10446** | VLESS | TCP+Reality | XTLS-Reality (SNI: `www.yahoo.com`, Flow: `none`) |
| **10554** | VMess | TCP+Reality | VMess Reality (SNI: `www.cisco.com`, Flow: `none`) |
| **10555** | VMess | XHTTP+Reality | VMess Reality (SNI: `www.speedtest.net`, Flow: `none`, path: `/vmess-xhttp-reality`) |
| **10556** | VMess | TCP+Reality | VMess Reality (SNI: `www.bing.com`, Flow: `none`) |
| **10664** | Trojan | TCP+Reality | Trojan Reality (SNI: `apple.com`, Flow: `none`) |
| **10665** | Trojan | XHTTP+Reality | Trojan Reality (SNI: `www.icloud.com`, Flow: `none`, path: `/trojan-xhttp-reality`) |
| **8080** | Nginx | HTTP | Camouflage Web + Proxying HTTP/1.1 & HTTP/2 (WS, gRPC, XHTTP) |

---

## ⚙️ 4. Format Konfigurasi (`config.yaml`)

Berikut adalah berkas konfigurasi `config.yaml` yang digunakan oleh `soft-proxy` untuk memetakan port tujuan secara terpusat berdasarkan SNI:

### A. Single-Domain Certs (Tanpa DNS API / HTTP-01 Challenge)

Dalam mode ini, `soft-proxy` mengajukan sertifikat Let's Encrypt untuk satu domain saja menggunakan verifikasi port 80 HTTP-01. Parameter `dns_provider` dan `cloudflare_token` dikosongkan/dihapus:

```yaml
bind_addr: "0.0.0.0"
http_port: 80
https_port: 443

acme:
  enabled: true
  domains:
    - "yourdomain.com" # Hanya 1 domain utama
  cache_dir: "/etc/soft-proxy/certs" # Direktori penyimpanan sertifikat SSL/TLS
  email: "admin@yourdomain.com" # Email terdaftar Let's Encrypt

backends:
  vmess: "127.0.0.1:1334"
  vless: "127.0.0.1:1234"
  trojan: "127.0.0.1:1434"
  http: "127.0.0.1:8080"

reality_backends:
  # VLESS Reality
  "127.0.0.1:10444":
    - "yahoo.com"
  "127.0.0.1:10445":
    - "www.google.com"
  "127.0.0.1:10446":
    - "www.yahoo.com"

  # VMess Reality
  "127.0.0.1:10554":
    - "www.cisco.com"
  "127.0.0.1:10555":
    - "www.speedtest.net"
  "127.0.0.1:10556":
    - "www.bing.com"

  # Trojan Reality
  "127.0.0.1:10664":
    - "apple.com"
  "127.0.0.1:10665":
    - "www.icloud.com"
```

### B. Single-Domain Certs (Dengan DNS API / DNS-01 Challenge)

Dalam mode ini, `soft-proxy` mengajukan sertifikat untuk satu domain menggunakan tantangan DNS-01 Cloudflare API. Tantangan diselesaikan secara otomatis di latar belakang tanpa membutuhkan port 80:

```yaml
bind_addr: "0.0.0.0"
http_port: 80
https_port: 443

acme:
  enabled: true
  domains:
    - "yourdomain.com" # Hanya 1 domain utama
  cache_dir: "/etc/soft-proxy/certs"
  dns_provider: "cloudflare" # Mengaktifkan verifikasi via Cloudflare DNS API
  email: "admin@yourdomain.com"
  cloudflare_token: "YOUR_CLOUDFLARE_API_TOKEN" # Token API Cloudflare

backends:
  vmess: "127.0.0.1:1334"
  vless: "127.0.0.1:1234"
  trojan: "127.0.0.1:1434"
  http: "127.0.0.1:8080"

reality_backends:
  # VLESS Reality
  "127.0.0.1:10444":
    - "yahoo.com"
  "127.0.0.1:10445":
    - "www.google.com"
  "127.0.0.1:10446":
    - "www.yahoo.com"

  # VMess Reality
  "127.0.0.1:10554":
    - "www.cisco.com"
  "127.0.0.1:10555":
    - "www.speedtest.net"
  "127.0.0.1:10556":
    - "www.bing.com"

  # Trojan Reality
  "127.0.0.1:10664":
    - "apple.com"
  "127.0.0.1:10665":
    - "www.icloud.com"
```

### C. Multi-Domain Certs (Tanpa DNS API / HTTP-01 Challenge)

Dalam mode ini, `soft-proxy` mengajukan sertifikat untuk beberapa domain berbeda secara bertahap menggunakan verifikasi HTTP-01. Skenario ini tidak mendukung domain wildcard:

```yaml
bind_addr: "0.0.0.0"
http_port: 80
https_port: 443

acme:
  enabled: true
  domains:
    - "yourdomain.com"    # Domain utama pertama
    - "anotherdomain.com" # Domain tambahan kedua
  cache_dir: "/etc/soft-proxy/certs"
  email: "admin@yourdomain.com"

backends:
  vmess: "127.0.0.1:1334"
  vless: "127.0.0.1:1234"
  trojan: "127.0.0.1:1434"
  http: "127.0.0.1:8080"

reality_backends:
  # VLESS Reality
  "127.0.0.1:10444":
    - "yahoo.com"
  "127.0.0.1:10445":
    - "www.google.com"
  "127.0.0.1:10446":
    - "www.yahoo.com"

  # VMess Reality
  "127.0.0.1:10554":
    - "www.cisco.com"
  "127.0.0.1:10555":
    - "www.speedtest.net"
  "127.0.0.1:10556":
    - "www.bing.com"

  # Trojan Reality
  "127.0.0.1:10664":
    - "apple.com"
  "127.0.0.1:10665":
    - "www.icloud.com"
```

### D. Multi-Domain Certs (Dengan DNS API / DNS-01 Challenge)

Dalam mode ini, `soft-proxy` mengajukan sertifikat untuk beberapa domain sekaligus secara otomatis lewat Cloudflare DNS API (termasuk dukungan penuh wildcard domain):

```yaml
bind_addr: "0.0.0.0"
http_port: 80
https_port: 443

acme:
  enabled: true
  domains:
    - "yourdomain.com"       # Domain utama pertama
    - "anotherdomain.com"    # Domain tambahan kedua
    - "*.wildcarddomain.com" # Contoh domain wildcard yang didukung
  cache_dir: "/etc/soft-proxy/certs"
  dns_provider: "cloudflare"
  email: "admin@yourdomain.com"
  cloudflare_token: "YOUR_CLOUDFLARE_API_TOKEN"

backends:
  vmess: "127.0.0.1:1334"
  vless: "127.0.0.1:1234"
  trojan: "127.0.0.1:1434"
  http: "127.0.0.1:8080"

reality_backends:
  # VLESS Reality
  "127.0.0.1:10444":
    - "yahoo.com"
  "127.0.0.1:10445":
    - "www.google.com"
  "127.0.0.1:10446":
    - "www.yahoo.com"

  # VMess Reality
  "127.0.0.1:10554":
    - "www.cisco.com"
  "127.0.0.1:10555":
    - "www.speedtest.net"
  "127.0.0.1:10556":
    - "www.bing.com"

  # Trojan Reality
  "127.0.0.1:10664":
    - "apple.com"
  "127.0.0.1:10665":
    - "www.icloud.com"
```

---

## 🚀 5. Kompilasi & Menjalankan Aplikasi

Kompilasi kode menggunakan Go compiler:
```bash
go build -o soft-proxy main.go
# Pindahkan biner ke lokasi standard sistem
mv soft-proxy /usr/local/bin/
```

Deploy sebagai systemd service di `/etc/systemd/system/soft-proxy.service` agar selalu menyala di latar belakang:
```ini
[Unit]
Description=Soft Proxy Multiplexer
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/etc/soft-proxy
ExecStart=/usr/local/bin/soft-proxy
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

Nyalakan layanannya:
```bash
systemctl daemon-reload
systemctl enable soft-proxy
systemctl start soft-proxy
```
Dapatkan log aktivitas koneksi secara live melalui:
```bash
tail -f /var/log/soft-proxy/soft-proxy.log
```

---

## 🔍 6. Contoh Tautan Impor Klien Xray (Client Import URL Examples)

Berikut adalah daftar lengkap **38 contoh tautan impor (*URL import links*)** untuk klien VPN (seperti v2rayNG, Nekobox, Shadowrocket, atau Xray CLI) agar terhubung ke server multiplexer `soft-proxy`:

### A. Tautan Klien Port 443 TLS (Sertifikat Let's Encrypt / ACME Valid)

| Skenario Protokol | Protokol | Keamanan | Transport | Tautan Impor Klien (Client Import URL) |
| :--- | :--- | :--- | :--- | :--- |
| **VLESS TCP + TLS** | VLESS | TLS | TCP | `vless://09abf07d-a8ea-4748-9f37-5b3dca0e0a94@yourdomain.com:443?encryption=none&security=tls&sni=yourdomain.com#VLESS-TCP-TLS` |
| **VLESS WebSocket + TLS** | VLESS | TLS | WS | `vless://09abf07d-a8ea-4748-9f37-5b3dca0e0a94@yourdomain.com:443?encryption=none&security=tls&sni=yourdomain.com&type=ws&path=%2Fvless-ws#VLESS-WS-TLS` |
| **VLESS HTTPUpgrade + TLS** | VLESS | TLS | HTTPUpgrade | `vless://09abf07d-a8ea-4748-9f37-5b3dca0e0a94@yourdomain.com:443?encryption=none&security=tls&sni=yourdomain.com&type=httpupgrade&path=%2Fvless-httpupgrade#VLESS-HTTPUpgrade-TLS` |
| **VLESS gRPC + TLS** | VLESS | TLS | gRPC | `vless://09abf07d-a8ea-4748-9f37-5b3dca0e0a94@yourdomain.com:443?encryption=none&security=tls&sni=yourdomain.com&type=grpc&serviceName=vless-grpc#VLESS-gRPC-TLS` |
| **VLESS XHTTP + TLS** | VLESS | TLS | XHTTP | `vless://09abf07d-a8ea-4748-9f37-5b3dca0e0a94@yourdomain.com:443?encryption=none&security=tls&sni=yourdomain.com&type=xhttp&path=%2Fvless-xhttp#VLESS-XHTTP-TLS` |
| **VMess TCP + TLS** | VMess | TLS | TCP | `vmess://eyJ2IjoiMiIsInBzIjoiVk1FU1MtVENQLVRMUyIsImFkZCI6InlvdXJkb21haW4uY29tIiwicG9ydCI6IjQ0MyIsImlkIjoiOGU0MmI0NzgtM2E0Yi00OGIwLWFkMmUtYTU4NjI1YWNkYThlIiwiYWlkIjoiMCIsIm5ldCI6InRjcCIsInR5cGUiOiJub25lIiwidGxzIjoidGxzIiwic25pIjoieW91cmRvbWFpbi5jb20ifQ` |
| **VMess WebSocket + TLS** | VMess | TLS | WS | `vmess://eyJ2IjoiMiIsInBzIjoiVk1FU1MtV1MtVExTIiwiYWRkIjoieW91cmRvbWFpbi5jb20iLCJwb3J0IjoiNDQzIiwiaWQiOiI4ZTQyYjQ3OC0zYTRiLTQ4YjAtYWQyZS1hNTg2MjVhY2RhOGUiLCJhaWQiOiIwIiwibmV0Ijoid3MiLCJ0eXBlIjoibm9uZSIsImhvc3QiOiJ5b3VyZG9tYWluLmNvbSIsInBhdGgiOiIvdm1lc3Mtd3MiLCJ0bHMiOiJ0bHMiLCJzbmkiOiJ5b3VyZG9tYWluLmNvbSJ9` |
| **VMess HTTPUpgrade + TLS** | VMess | TLS | HTTPUpgrade | `vmess://eyJ2IjoiMiIsInBzIjoiVk1FU1MtSFRUUFVwLVRMUyIsImFkZCI6InlvdXJkb21haW4uY29tIiwicG9ydCI6IjQ0MyIsImlkIjoiOGU0MmI0NzgtM2E0Yi00OGIwLWFkMmUtYTU4NjI1YWNkYThlIiwiYWlkIjoiMCIsIm5ldCI6Imh0dHB1cGdyYWRlIiwidHlwZSI6Im5vbmUiLCJob3N0IjoieW91cmRvbWFpbi5jb20iLCJwYXRoIjoiL3ZtZXNzLWh0dHB1cGdyYWRlIiwidGxzIjoidGxzIiwic25pIjoieW91cmRvbWFpbi5jb20ifQ` |
| **VMess gRPC + TLS** | VMess | TLS | gRPC | `vmess://eyJ2IjoiMiIsInBzIjoiVk1FU1MtZ1JQQy1UTFMiLCJhZGQiOiJ5b3VyZG9tYWluLmNvbSIsInBvcnQiOiI0NDMiLCJpZCI6IjhlNDJiNDc4LTNhNGItNDhiMC1hZDJlLWE1ODYyNWFjZGE4ZSIsImFpZCI6IjAiLCJuZXQiOiJncnBjIiwidHlwZSI6Im5vbmUiLCJwYXRoIjoidm1lc3MtZ3JwYyIsInRscyI6InRscyIsInNuaSI6InlvdXJkb21haW4uY29tIn0` |
| **VMess XHTTP + TLS** | VMess | TLS | XHTTP | `vmess://eyJ2IjoiMiIsInBzIjoiVk1FU1MtWUhUVFAtVExTIiwiYWRkIjoieW91cmRvbWFpbi5jb20iLCJwb3J0IjoiNDQzIiwiaWQiOiI4ZTQyYjQ3OC0zYTRiLTQ4YjAtYWQyZS1hNTg2MjVhY2RhOGUiLCJhaWQiOiIwIiwibmV0IjoieGh0dHAiLCJ0eXBlIjoibm9uZSIsImhvc3QiOiJ5b3VyZG9tYWluLmNvbSIsInBhdGgiOiIvdm1lc3MteGh0dHAiLCJ0bHMiOiJ0bHMiLCJzbmkiOiJ5b3VyZG9tYWluLmNvbSJ9` |
| **Trojan TCP + TLS** | Trojan | TLS | TCP | `trojan://140141d26c7dfa2171cf1cc460190ba2@yourdomain.com:443?security=tls&type=tcp&sni=yourdomain.com#Trojan-TCP-TLS` |
| **Trojan WebSocket + TLS** | Trojan | TLS | WS | `trojan://140141d26c7dfa2171cf1cc460190ba2@yourdomain.com:443?security=tls&type=ws&path=%2Ftrojan-ws&host=yourdomain.com&sni=yourdomain.com#Trojan-WS-TLS` |
| **Trojan HTTPUpgrade + TLS** | Trojan | TLS | HTTPUpgrade | `trojan://140141d26c7dfa2171cf1cc460190ba2@yourdomain.com:443?security=tls&type=httpupgrade&path=%2Ftrojan-httpupgrade&host=yourdomain.com#Trojan-HTTPUpgrade-TLS` |
| **Trojan gRPC + TLS** | Trojan | TLS | gRPC | `trojan://140141d26c7dfa2171cf1cc460190ba2@yourdomain.com:443?security=tls&type=grpc&serviceName=trojan-grpc&sni=yourdomain.com#Trojan-gRPC-TLS` |
| **Trojan XHTTP + TLS** | Trojan | TLS | XHTTP | `trojan://140141d26c7dfa2171cf1cc460190ba2@yourdomain.com:443?security=tls&type=xhttp&path=%2Ftrojan-xhttp&host=yourdomain.com#Trojan-XHTTP-TLS` |

### B. Tautan Klien Port 443 Reality (TLS Bypass)

| Skenario Protokol | Protokol | Keamanan | Transport | Tautan Impor Klien (Client Import URL) |
| :--- | :--- | :--- | :--- | :--- |
| **VLESS Reality TCP Vision** | VLESS | Reality | TCP (Vision) | `vless://e75a1d12-7c68-4971-b1fb-3f7fe767c6d6@yourdomain.com:443?encryption=none&security=reality&sni=yahoo.com&fp=chrome&pbk=Y07pOrSNdp7YtiCXffp64UoTanx1J4LK_YX8HkHs_is&sid=01234567&flow=xtls-rprx-vision#VLESS-Reality-Vision` |
| **VLESS Reality TCP (No Flow)** | VLESS | Reality | TCP | `vless://09abf07d-a8ea-4748-9f37-5b3dca0e0a94@yourdomain.com:443?encryption=none&security=reality&sni=www.yahoo.com&fp=chrome&pbk=Y07pOrSNdp7YtiCXffp64UoTanx1J4LK_YX8HkHs_is&sid=01234567#VLESS-Reality-None` |
| **VLESS Reality XHTTP** | VLESS | Reality | XHTTP | `vless://09abf07d-a8ea-4748-9f37-5b3dca0e0a94@yourdomain.com:443?encryption=none&security=reality&sni=www.google.com&fp=chrome&pbk=Y07pOrSNdp7YtiCXffp64UoTanx1J4LK_YX8HkHs_is&sid=01234567&type=xhttp&path=%2Fvless-xhttp-reality#VLESS-Reality-XHTTP` |
| **VMess Reality TCP Vision** | VMess | Reality | TCP (Vision) | `vmess://eyJ2IjoiMiIsInBzIjoiVk1FU1MtUmVhbGl0eS1UQ1AtVmlzaW9uIiwiYWRkIjoieW91cmRvbWFpbi5jb20iLCJwb3J0IjoiNDQzIiwiaWQiOiI4ZTQyYjQ3OC0zYTRiLTQ4YjAtYWQyZS1hNTg2MjVhY2RhOGUiLCJhaWQiOiIwIiwibmV0IjoidGNwIiwidHlwZSI6Im5vbmUiLCJ0bHMiOiJyZWFsaXR5Iiwic25pIjoid3d3LmNpc2NvLmNvbSIsInBiayI6IlkwNHBPclNOZHA3WXRpQ1hmZnA2NFVvVGFueDFINExLX1lYOEhrSHNfaXMiLCJzaWQiOiIwMTIzNDU2NyIsImZwIjoiY2hyb21lIiwiZmxvdyI6Inh0bHMtcnByaC12aXNpb24ifQ` |
| **VMess Reality TCP (No Flow)** | VMess | Reality | TCP | `vmess://eyJ2IjoiMiIsInBzIjoiVk1FU1MtUmVhbGl0eS1UQ1AiLCJhZGQiOiJ5b3VyZG9tYWluLmNvbSIsInBvcnQiOiI0NDMiLCJpZCI6IjhlNDJiNDc4LTNhNGItNDhiMC1hZDJlLWE1ODYyNWFjZGE4ZSIsImFpZCI6IjAiLCJuZXQiOiJ0Y3AiLCJ0eXBlIjoibm9uZSIsInRscyI6InJlYWxpdHkiLCJzbmkiOiJ3d3cuYmluZy5jb20iLCJwYmsiOiJZMDdwT3JTTmRwN1l0aUNYZmZwNjRVb1RhbngxSjRMS19ZWDhIa0hzX2lzIiwic2lkIjoiMDEyMzQ1NjciLCJmcCI6ImNocm9tZSJ9` |
| **VMess Reality XHTTP** | VMess | Reality | XHTTP | `vmess://eyJ2IjoiMiIsInBzIjoiVk1FU1MtUmVhbGl0eS1YSFRUUCIsImFkZCI6InlvdXJkb21haW4uY29tIiwicG9ydCI6IjQ0MyIsImlkIjoiOGU0MmI0NzgtM2E0Yi00OGIwLWFkMmUtYTU4NjI1YWNkYThlIiwiYWlkIjoiMCIsIm5ldCI6InhodHRwIiwidHlwZSI6Im5vbmUiLCJ0bHMiOiJyZWFsaXR5Iiwic25pIjoid3d3LnNwZWVkdGVzdC5uZXQiLCJwYmsiOiJZMDdwT3JTTmRwN1l0aUNYZmZwNjRVb1RhbngxSjRMS19ZWDhIa0hzX2lzIiwic2lkIjoiMDEyMzQ1NjciLCJmcCI6ImNocm9tZSIsInBhdGgiOiIvdm1lc3MteGh0dHAtcmVhbGl0eSJ9` |
| **Trojan Reality TCP** | Trojan | Reality | TCP | `trojan://140141d26c7dfa2171cf1cc460190ba2@yourdomain.com:443?security=reality&type=tcp&pbk=Y07pOrSNdp7YtiCXffp64UoTanx1J4LK_YX8HkHs_is&sni=apple.com&fp=chrome&sid=01234567#Trojan-Reality-TCP` |
| **Trojan Reality XHTTP** | Trojan | Reality | XHTTP | `trojan://140141d26c7dfa2171cf1cc460190ba2@yourdomain.com:443?security=reality&type=xhttp&path=%2Ftrojan-xhttp-reality&pbk=Y07pOrSNdp7YtiCXffp64UoTanx1J4LK_YX8HkHs_is&sni=www.icloud.com&fp=chrome&sid=01234567#Trojan-Reality-XHTTP` |

### C. Tautan Klien Port 80 Plain (No TLS)

| Skenario Protokol | Protokol | Keamanan | Transport | Tautan Impor Klien (Client Import URL) |
| :--- | :--- | :--- | :--- | :--- |
| **VLESS TCP Plain** | VLESS | None | TCP | `vless://09abf07d-a8ea-4748-9f37-5b3dca0e0a94@yourdomain.com:80?encryption=none&security=none#VLESS-TCP-Plain` |
| **VLESS WS Plain** | VLESS | None | WS | `vless://09abf07d-a8ea-4748-9f37-5b3dca0e0a94@yourdomain.com:80?encryption=none&security=none&type=ws&path=%2Fvless-ws#VLESS-WS-Plain` |
| **VLESS HTTPUpgrade Plain** | VLESS | None | HTTPUpgrade | `vless://09abf07d-a8ea-4748-9f37-5b3dca0e0a94@yourdomain.com:80?encryption=none&security=none&type=httpupgrade&path=%2Fvless-httpupgrade#VLESS-HTTPUpgrade-Plain` |
| **VLESS gRPC Plain** | VLESS | None | gRPC | `vless://09abf07d-a8ea-4748-9f37-5b3dca0e0a94@yourdomain.com:80?encryption=none&security=none&type=grpc&serviceName=vless-grpc#VLESS-gRPC-Plain` |
| **VLESS XHTTP Plain** | VLESS | None | XHTTP | `vless://09abf07d-a8ea-4748-9f37-5b3dca0e0a94@yourdomain.com:80?encryption=none&security=none&type=xhttp&path=%2Fvless-xhttp#VLESS-XHTTP-Plain` |
| **VMess TCP Plain** | VMess | None | TCP | `vmess://eyJ2IjoiMiIsInBzIjoiVk1FU1MtVENQLVBsYWluIiwiYWRkIjoieW91cmRvbWFpbi5jb20iLCJwb3J0IjoiODAiLCJpZCI6IjhlNDJiNDc4LTNhNGItNDhiMC1hZDJlLWE1ODYyNWFjZGE4ZSIsImFpZCI6IjAiLCJuZXQiOiJ0Y3AiLCJ0eXBlIjoibm9uZSIsInRscyI6Im5vbmUifQ` |
| **VMess WS Plain** | VMess | None | WS | `vmess://eyJ2IjoiMiIsInBzIjoiVk1FU1MtV1MtUGxhaW4iLCJhZGQiOiJ5b3VyZG9tYWluLmNvbSIsInBvcnQiOiI4MCIsImlkIjoiOGU0MmI0NzgtM2E0Yi00OGIwLWFkMmUtYTU4NjI1YWNkYThlIiwiYWlkIjoiMCIsIm5ldCI6IndzIiwidHlwZSI6Im5vbmUiLCJob3N0IjoieW91cmRvbWFpbi5jb20iLCJwYXRoIjoiL3ZtZXNzLXdzIiwidGxzIjoibm9uZSJ9` |
| **VMess HTTPUpgrade Plain** | VMess | None | HTTPUpgrade | `vmess://eyJ2IjoiMiIsInBzIjoiVk1FU1MtSFRUUFVwLVBsYWluIiwiYWRkIjoieW91cmRvbWFpbi5jb20iLCJwb3J0IjoiODAiLCJpZCI6IjhlNDJiNDc4LTNhNGItNDhiMC1hZDJlLWE1ODYyNWFjZGE4ZSIsImFpZCI6IjAiLCJuZXQiOiJodHRwdXBncmFkZSIsInR5cGUiOiJub25lIiwiaG9zdCI6InlvdXJkb21haW4uY29tIiwicGF0aCI6Ii92bWVzcy1odHRwdXBncmFkZSIsInRscyI6Im5vbmUifQ` |
| **VMess gRPC Plain** | VMess | None | gRPC | `vmess://eyJ2IjoiMiIsInBzIjoiVk1FU1MtZ1JQQy1QbGFpbiIsImFkZCI6InlvdXJkb21haW4uY29tIiwicG9ydCI6IjgwIiwiaWQiOiI4ZTQyYjQ3OC0zYTRiLTQ4YjAtYWQyZS1hNTg2MjVhY2RhOGUiLCJhaWQiOiIwIiwibmV0IjoiZ3JwYyIsInR5cGUiOiJub25lIiwicGF0aCI6InZtZXNzLWdycGMiLCJ0bHMiOiJub25lIn0` |
| **VMess XHTTP Plain** | VMess | None | XHTTP | `vmess://eyJ2IjoiMiIsInBzIjoiVk1FU1MtWUhUVFAtUGxhaW4iLCJhZGQiOiJ5b3VyZG9tYWluLmNvbSIsInBvcnQiOiI4MCIsImlkIjoiOGU0MmI0NzgtM2E0Yi00OGIwLWFkMmUtYTU4NjI1YWNkYThlIiwiYWlkIjoiMCIsIm5ldCI6InhodHRwIiwidHlwZSI6Im5vbmUiLCJob3N0IjoieW91cmRvbWFpbi5jb20iLCJwYXRoIjoiL3ZtZXNzLXhodHRwIiwidGxzIjoibm9uZSJ9` |
| **Trojan TCP Plain** | Trojan | None | TCP | `trojan://140141d26c7dfa2171cf1cc460190ba2@yourdomain.com:80?security=none&type=tcp#Trojan-TCP-Plain` |
| **Trojan WS Plain** | Trojan | None | WS | `trojan://140141d26c7dfa2171cf1cc460190ba2@yourdomain.com:80?security=none&type=ws&path=%2Ftrojan-ws&host=yourdomain.com#Trojan-WS-Plain` |
| **Trojan HTTPUpgrade Plain** | Trojan | None | HTTPUpgrade | `trojan://140141d26c7dfa2171cf1cc460190ba2@yourdomain.com:80?security=none&type=httpupgrade&path=%2Ftrojan-httpupgrade&host=yourdomain.com#Trojan-HTTPUpgrade-Plain` |
| **Trojan gRPC Plain** | Trojan | None | gRPC | `trojan://140141d26c7dfa2171cf1cc460190ba2@yourdomain.com:80?security=none&type=grpc&serviceName=trojan-grpc#Trojan-gRPC-Plain` |
| **Trojan XHTTP Plain** | Trojan | None | XHTTP | `trojan://140141d26c7dfa2171cf1cc460190ba2@yourdomain.com:80?security=none&type=xhttp&path=%2Ftrojan-xhttp&host=yourdomain.com#Trojan-XHTTP-Plain` |

---

## ⚙️ 7. Contoh Berkas Konfigurasi Klien Xray (Client JSON Config Examples)

Bagi pengguna yang ingin menjalankan aplikasi Xray secara langsung di sisi klien dengan berkas JSON (`config.json`), berikut adalah contoh konfigurasi siap pakai:

### A. Contoh Klien VLESS WebSocket + TLS
```json
{
  "log": {
    "loglevel": "warning"
  },
  "inbounds": [
    {
      "port": 10808,
      "listen": "127.0.0.1",
      "protocol": "socks",
      "settings": {
        "auth": "noauth",
        "udp": true
      }
    }
  ],
  "outbounds": [
    {
      "protocol": "vless",
      "settings": {
        "vnext": [
          {
            "address": "yourdomain.com",
            "port": 443,
            "users": [
              {
                "id": "09abf07d-a8ea-4748-9f37-5b3dca0e0a94",
                "encryption": "none"
              }
            ]
          }
        ]
      },
      "streamSettings": {
        "network": "ws",
        "security": "tls",
        "tlsSettings": {
          "serverName": "yourdomain.com"
        },
        "wsSettings": {
          "path": "/vless-ws",
          "headers": {
            "Host": "yourdomain.com"
          }
        }
      }
    }
  ]
}
```

### B. Contoh Klien VLESS Reality TCP Vision (XTLS)
```json
{
  "log": {
    "loglevel": "warning"
  },
  "inbounds": [
    {
      "port": 10808,
      "listen": "127.0.0.1",
      "protocol": "socks",
      "settings": {
        "auth": "noauth",
        "udp": true
      }
    }
  ],
  "outbounds": [
    {
      "protocol": "vless",
      "settings": {
        "vnext": [
          {
            "address": "yourdomain.com",
            "port": 443,
            "users": [
              {
                "id": "e75a1d12-7c68-4971-b1fb-3f7fe767c6d6",
                "encryption": "none",
                "flow": "xtls-rprx-vision"
              }
            ]
          }
        ]
      },
      "streamSettings": {
        "network": "tcp",
        "security": "reality",
        "realitySettings": {
          "fingerprint": "chrome",
          "serverName": "yahoo.com",
          "publicKey": "Y07pOrSNdp7YtiCXffp64UoTanx1J4LK_YX8HkHs_is",
          "shortId": "01234567",
          "spiderX": "/"
        }
      }
    }
  ]
}
```

### C. Contoh Klien VMess TCP + TLS
```json
{
  "log": {
    "loglevel": "warning"
  },
  "inbounds": [
    {
      "port": 10808,
      "listen": "127.0.0.1",
      "protocol": "socks",
      "settings": {
        "auth": "noauth",
        "udp": true
      }
    }
  ],
  "outbounds": [
    {
      "protocol": "vmess",
      "settings": {
        "vnext": [
          {
            "address": "yourdomain.com",
            "port": 443,
            "users": [
              {
                "id": "8e42b478-3a4b-48b0-ad2e-a58625acda8e",
                "alterId": 0
              }
            ]
          }
        ]
      },
      "streamSettings": {
        "network": "tcp",
        "security": "tls",
        "tlsSettings": {
          "serverName": "yourdomain.com"
        }
      }
    }
  ]
}
```

### D. Contoh Klien Trojan gRPC + TLS
```json
{
  "log": {
    "loglevel": "warning"
  },
  "inbounds": [
    {
      "port": 10808,
      "listen": "127.0.0.1",
      "protocol": "socks",
      "settings": {
        "auth": "noauth",
        "udp": true
      }
    }
  ],
  "outbounds": [
    {
      "protocol": "trojan",
      "settings": {
        "servers": [
          {
            "address": "yourdomain.com",
            "port": 443,
            "password": "140141d26c7dfa2171cf1cc460190ba2"
          }
        ]
      },
      "streamSettings": {
        "network": "grpc",
        "security": "tls",
        "tlsSettings": {
          "serverName": "yourdomain.com"
        },
        "grpcSettings": {
          "serviceName": "trojan-grpc"
        }
      }
    }
  ]
}
```

---

## 📄 Lisensi (License)

Proyek ini dilisensikan di bawah **MIT License**. Lihat berkas [LICENSE](file:///root/proyek/soft/LICENSE) untuk detail selengkapnya.




