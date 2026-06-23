# Soft-Proxy: Multiplexer & Account Manager

Soft-Proxy adalah sistem manajemen proxy terintegrasi yang menggabungkan layanan **Multiplexer Server** (port-sharing) dan **CLI Menu Account Manager** berbasis bahasa Go. 

Sistem ini didesain khusus untuk dijalankan di VPS linux (termasuk terminal seluler termux/SSH) dengan tingkat kehandalan tinggi, keamanan dari serangan Denial of Service (DoS), dan kemudahan pengelolaan akun klien secara dinamis melalui gRPC API ke Xray core.

---

## 📂 Struktur Proyek (Project Structure)

Berikut adalah struktur folder dan berkas proyek Soft-Proxy beserta penjelasannya:

```text
soft/
├── cmd/
│   ├── soft-proxy/            # Kode utama layanan server multiplexer
│   │   └── main.go
│   └── soft-menu/             # Kode utama aplikasi CLI Menu Manager (bin/menu)
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
│   ├── db/                    # Modul database SQLite (CGO-free) untuk penyimpanan klien
│   │   └── db.go
│   ├── logger/                # Logging JSON & rotasi otomatis ukuran berkas log
│   │   └── logger.go
│   └── xrayapi/               # Handler gRPC API untuk memanipulasi inbound Xray core
│       └── api.go
├── config.yaml                # Berkas konfigurasi utama untuk backends & domain
├── go.mod                     # Go Modules file dependensi
├── go.sum                     # Checksum dependensi Go
└── .gitignore                 # Daftar berkas yang diabaikan oleh Git (DB, certs, log)
```

---

## 🏗️ Arsitektur & Cara Kerja (Architecture & Multiplexing)

### 1. Multiplexing & SNI Sniffing (Port 443 & 80 Sharing)
*   **Port 80 (HTTP):** Mendeteksi tantangan ACME (`/.well-known/acme-challenge/`). Jika bukan ACME, koneksi akan diredirect secara otomatis ke port HTTPS (443).
*   **Port 443 (HTTPS):** Mendeteksi koneksi TLS masuk tanpa melakukan terminasi jabat tangan (handshake) terlebih dahulu. `soft-proxy` membaca ClientHello untuk mengidentifikasi **Server Name Indication (SNI)**.
    *   Jika SNI terdaftar sebagai **Reality Domain**, koneksi di-bypass (piping mentah) langsung ke port Xray Reality tanpa membongkar enkripsi.
    *   Jika SNI merupakan domain standar, handshake TLS diselesaikan oleh `soft-proxy` (menggunakan sertifikat Let's Encrypt/Self-signed). Setelah didekripsi, muatan di-sniff untuk mendeteksi protokol (`vmess`, `vless`, `trojan`, atau `http` biasa) dan diteruskan ke backend Xray yang sesuai.

### 2. Auto-Blocker Probing Port
Jika ada pemindaian (*probing*) port ilegal atau kegagalan *TLS Handshake* lebih dari 10 kali dalam 1 menit dari IP yang sama, sistem auto-blocker akan mengisolasi IP tersebut selama 1 jam.

### 3. CLI Account Manager & Xray gRPC API
Aplikasi `soft-menu` bertindak sebagai antarmuka admin untuk:
1.  Menyimpan detail akun klien di database SQLite (`/etc/soft-proxy/panel.db`).
2.  Menyuntikkan (*inject*) UUID/Kredensial klien secara dinamis ke runtime memori Xray via gRPC API tanpa perlu melakukan restart daemon Xray.

---

## ⚙️ Konfigurasi (`config.yaml`)

Pemuatan konfigurasi bersifat *hot-reload* (langsung aktif begitu file disimpan tanpa restart biner).

```yaml
bind_addr: "0.0.0.0"
http_port: 80
https_port: 443

# Sertifikat bawaan (jika ACME tidak aktif)
cert_file: "/root/proyek/soft/certs/selfsigned.crt"
key_file: "/root/proyek/soft/certs/selfsigned.key"

acme:
  enabled: true
  domains:
    - "test2.tunnel.sryze.cc"
  cache_dir: "./certs"
  dns_provider: "cloudflare" # cloudflare atau http
  email: "admin@sryze.cc"
  cloudflare_token: "YOUR_CLOUDFLARE_API_TOKEN"

backends:
  http: "127.0.0.1:80"        # Fallback web server
  vless: "127.0.0.1:1234"      # Xray VLESS Inbound
  vmess: "127.0.0.1:1334"      # Xray VMess Inbound
  trojan: "127.0.0.1:1434"     # Xray Trojan Inbound
  reality: "127.0.0.1:10444"   # Xray Reality Inbound

# Domain yang langsung di-bypass ke Reality
reality_domains:
  - "yahoo.com"
  - "www.google.com"
  - "www.speedtest.net"
  - "www.bing.com"
  - "apple.com"
  - "www.icloud.com"
```

---

## 🚀 Instalasi & Menjalankan Aplikasi

### 1. Kompilasi Kode
Kompilasi kedua aplikasi menggunakan Go compiler:
```bash
# Kompilasi Multiplexer
go build -o bin/soft-proxy cmd/soft-proxy/main.go

# Kompilasi CLI Manager
go build -o bin/soft-menu cmd/soft-menu/main.go
```

### 2. Pemasangan Biner CLI Hapus/Tambah Akun
Pasang biner menu ke path system agar dapat dipanggil langsung dengan perintah `menu`:
```bash
cp bin/soft-menu /usr/local/bin/menu
chmod +x /usr/local/bin/menu
```
Sekarang, administrator VPS cukup mengetik `menu` di terminal SSH untuk mengelola VPN.

### 3. Deploy soft-proxy sebagai Systemd Service
Untuk memastikan server multiplexer berjalan 24/7 di latar belakang dan menyala otomatis saat VPS dinyalakan kembali, buat berkas `/etc/systemd/system/soft-proxy.service`:

```ini
[Unit]
Description=Soft Proxy Multiplexer
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/root/proyek/soft
ExecStart=/root/proyek/soft/bin/soft-proxy
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

Aktifkan dan nyalakan service:
```bash
systemctl daemon-reload
systemctl enable soft-proxy
systemctl start soft-proxy
```

---

## 📈 Pemeliharaan & Debugging
Untuk memantau aktivitas koneksi masuk dan mendeteksi IP yang diblokir oleh auto-blocker secara real-time, periksa berkas log:
```bash
tail -f /var/log/soft-proxy/soft-proxy.log
```
Untuk memantau status systemd service:
```bash
systemctl status soft-proxy
```
